package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"prods/internal/bulkops"
	"prods/internal/catalog"
	"prods/internal/identity"
	"prods/internal/publishing"
	"prods/internal/storage/sqlite"
)

var errProductBulkPublisherUnavailable = errors.New("public publication coordinator is unavailable")

func (s *Server) adminPrepareProductBulk(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	var request bulkops.Request
	if err := decodeJSON(r.Body, &request); err != nil {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeInvalidJSON)
		return
	}
	run, err := s.store.PrepareProductBulk(r.Context(), current.UserID, request)
	if err != nil {
		s.writeProductBulkError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) adminProductBulk(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, false)
	if !ok {
		return
	}
	run, err := s.store.ProductBulkRun(r.Context(), current.UserID, r.PathValue("id"))
	if err != nil {
		s.writeProductBulkError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) adminExecuteProductBulk(w http.ResponseWriter, r *http.Request) {
	current, ok := s.requireCapability(w, r, identity.CapabilityCatalogEdit, true)
	if !ok {
		return
	}
	reservation, err := s.admitResource(r.Context(), "Product Bulk execution", s.config.DatabasePath, 8<<20, 4)
	if err != nil {
		s.writeResourceError(w, r, err)
		return
	}
	defer reservation.Release()
	run, err := s.store.ProductBulkRun(r.Context(), current.UserID, r.PathValue("id"))
	if err != nil {
		s.writeProductBulkError(w, r, err)
		return
	}
	if run.Status == "completed" && run.Receipt != nil {
		receipt := *run.Receipt
		receipt.Replay = true
		writeJSON(w, http.StatusOK, receipt)
		return
	}
	if err := s.store.StartProductBulk(r.Context(), current.UserID, run.Preview.RunID); err != nil {
		s.writeProductBulkError(w, r, err)
		return
	}
	for _, item := range run.Preview.Items {
		if _, exists, err := s.store.ProductBulkItemResult(r.Context(), run.Preview.RunID, item.ProductID); err != nil {
			s.writeProductBulkError(w, r, err)
			return
		} else if exists {
			continue
		}
		result := s.executeProductBulkItem(r, current.UserID, run.Preview, item)
		if _, err := s.store.RecordProductBulkItem(r.Context(), current.UserID, run.Preview.RunID, result); err != nil {
			s.writeProductBulkError(w, r, err)
			return
		}
	}
	receipt, err := s.store.CompleteProductBulk(r.Context(), current.UserID, run.Preview.RunID)
	if err != nil {
		s.writeProductBulkError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) executeProductBulkItem(r *http.Request, actorID string, preview bulkops.Preview, item bulkops.PlanItem) bulkops.ItemResult {
	result := bulkops.ItemResult{
		SelectionIndex: item.SelectionIndex, ProductID: item.ProductID, PartNumber: item.PartNumber,
		ExpectedRevision: item.ExpectedRevision,
	}
	if !item.Eligible {
		result.Status = bulkops.ItemInvalid
		result.Message = item.Message
		return result
	}
	current, err := s.store.Product(r.Context(), item.ProductID)
	if err != nil {
		result.Status, result.Message = classifyBulkItemError(err)
		return result
	}
	if current.Revision != item.ExpectedRevision {
		if current.Revision == item.ExpectedRevision+1 && bulkTargetReached(current, preview) {
			result.Status = bulkops.ItemSucceeded
			result.ResultRevision = current.Revision
			result.Message = "Recovered the durable result of an earlier execution attempt."
			result.PublicState = bulkPublicState(preview.Action)
			return result
		}
		result.Status = bulkops.ItemConflict
		result.ResultRevision = current.Revision
		result.Message = "Product revision changed after preflight; run preflight again."
		return result
	}
	if bulkTargetReached(current, preview) {
		result.Status = bulkops.ItemNoChange
		result.ResultRevision = current.Revision
		result.Message = "Product already has the requested value."
		result.PublicState = bulkPublicState(preview.Action)
		return result
	}

	var changed bool
	switch preview.Action {
	case bulkops.ActionHide:
		if s.publisher == nil {
			err = errProductBulkPublisherUnavailable
		} else {
			err = s.publisher.Revoke(r.Context(), publishing.RevokeRequest{
				ActorID: actorID, ProductID: item.ProductID, ExpectedRevision: item.ExpectedRevision,
			})
			changed = err == nil
		}
	case bulkops.ActionArchive:
		if s.publisher == nil {
			err = errProductBulkPublisherUnavailable
		} else {
			err = s.publisher.Revoke(r.Context(), publishing.RevokeRequest{
				ActorID: actorID, ProductID: item.ProductID, ExpectedRevision: item.ExpectedRevision, Archive: true,
			})
			changed = err == nil
		}
	case bulkops.ActionPublish, bulkops.ActionChangeCategory, bulkops.ActionChangeLifecycle:
		current, changed, err = s.store.ApplyProductBulkEdit(r.Context(), actorID, item.ProductID, item.ExpectedRevision, preview.Action, preview.TargetID)
		if err == nil {
			result.ResultRevision = current.Revision
		}
	default:
		err = bulkops.ErrInvalidRequest
	}
	if err != nil {
		result.Status, result.Message = classifyBulkItemError(err)
		return result
	}
	if result.ResultRevision == 0 {
		if current, readErr := s.store.Product(r.Context(), item.ProductID); readErr == nil {
			result.ResultRevision = current.Revision
		}
	}
	if changed {
		result.Status = bulkops.ItemSucceeded
	} else {
		result.Status = bulkops.ItemNoChange
	}
	result.PublicState = bulkPublicState(preview.Action)
	return result
}

func bulkTargetReached(product catalog.Product, preview bulkops.Preview) bool {
	switch preview.Action {
	case bulkops.ActionPublish:
		return product.RecordState == catalog.RecordCurrent && product.Status == catalog.Published
	case bulkops.ActionHide:
		return product.RecordState == catalog.RecordCurrent && product.Status == catalog.Hidden
	case bulkops.ActionArchive:
		return product.RecordState == catalog.RecordArchived && product.Status == catalog.Hidden
	case bulkops.ActionChangeCategory:
		return product.RecordState == catalog.RecordCurrent && product.CategoryID == preview.TargetID
	case bulkops.ActionChangeLifecycle:
		return product.RecordState == catalog.RecordCurrent && product.LifecycleID == preview.TargetID
	default:
		return false
	}
}

func bulkPublicState(action bulkops.Action) string {
	if action == bulkops.ActionHide || action == bulkops.ActionArchive {
		return "revoked"
	}
	return "publication_queued"
}

func classifyBulkItemError(err error) (bulkops.ItemStatus, string) {
	switch {
	case errors.Is(err, catalog.ErrRevisionConflict):
		return bulkops.ItemConflict, "Product revision changed after preflight; run preflight again."
	case errors.Is(err, catalog.ErrArchivedProduct), errors.Is(err, catalog.ErrInvalidProduct),
		errors.Is(err, catalog.ErrInvalidCategory), errors.Is(err, catalog.ErrDisabledReference),
		errors.Is(err, catalog.ErrRouteConflict), errors.Is(err, publishing.ErrSiteRouteInvalid):
		return bulkops.ItemInvalid, err.Error()
	default:
		message := strings.TrimSpace(err.Error())
		if len(message) > 300 {
			message = message[:300]
		}
		return bulkops.ItemFailed, message
	}
}

func (s *Server) writeProductBulkError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.writeAPIError(w, r, http.StatusNotFound, apiCodeNotFound)
	case errors.Is(err, sqlite.ErrPermissionDenied):
		s.writeAPIError(w, r, http.StatusForbidden, apiCodeForbidden)
	case errors.Is(err, bulkops.ErrRunConflict), errors.Is(err, catalog.ErrRevisionConflict):
		s.writeAPIError(w, r, http.StatusConflict, apiCodeRevisionConflict)
	case sqlite.IsProductBulkError(err), errors.Is(err, catalog.ErrDisabledReference):
		s.writeAPIError(w, r, http.StatusUnprocessableEntity, apiCodeValidationFailed)
	default:
		s.internalAPIError(w, r, err)
	}
}
