package publishing

import (
	"context"
	"errors"
	"maps"
	"sort"
	"strings"
)

// Optional so repositories and coordinators implementing the original full
// installation contract keep working. Website switches always use that path.
type productVisibilityReader interface {
	ActivePublication(context.Context, string) (ActivePublication, bool, error)
	ActiveProductRouteHistory(context.Context, string) ([]RouteHistory, error)
}

// InstallProductVisibility runs under the same post-commit admission gate as a
// full install. Only changed immutable files and asset references are checked.
func (e *Engine) InstallProductVisibility(ctx context.Context, productID string) error {
	if e.deferVisibility.Load() {
		e.deferredVisibilityDirty.Store(true)
		return nil
	}
	repository, ok := e.repository.(productVisibilityReader)
	if !ok || e.index == nil || e.index.unhealthy != nil || e.index.assetReferences == nil {
		return e.installLocked(ctx)
	}
	fail := func(err error) error {
		e.index = &publicIndex{unhealthy: err}
		return err
	}
	_, _, epoch, err := e.repository.ActiveWebsiteConfiguration(ctx)
	if err != nil {
		return fail(err)
	}
	disabled, generation, err := e.repository.WebsiteCustomCSSState(ctx)
	if err != nil {
		return fail(err)
	}
	if epoch != e.index.siteEpoch || disabled != e.index.customCSSDisabled || generation != e.index.runtimeGeneration {
		return e.installLocked(ctx)
	}
	activation, found, err := repository.ActivePublication(ctx, productID)
	if err != nil {
		return fail(err)
	}
	var assets []assetTarget
	if found {
		if activation.SiteEpoch != epoch {
			return e.installLocked(ctx)
		}
		if activation.ProductID != productID {
			return fail(errors.New("active publication does not match Product"))
		}
		if err := e.validateUnit(activation); err != nil {
			return fail(err)
		}
		assets, err = e.assetsForView(ctx, activation.View, false)
		if err != nil {
			return fail(err)
		}
	}
	history, err := repository.ActiveProductRouteHistory(ctx, productID)
	if err != nil {
		return fail(err)
	}
	next := *e.index
	next.byID = maps.Clone(next.byID)
	next.byRoute = maps.Clone(next.byRoute)
	next.byAsset = maps.Clone(next.byAsset)
	next.assetReferences = maps.Clone(next.assetReferences)
	next.productAssets = maps.Clone(next.productAssets)
	next.history = maps.Clone(next.history)
	if old, exists := next.byID[productID]; exists {
		delete(next.byID, productID)
		for _, suffix := range []string{"", ".json", ".md"} {
			delete(next.byRoute, old.Route+suffix)
		}
	}
	for _, id := range next.productAssets[productID] {
		next.assetReferences[id]--
		if next.assetReferences[id] == 0 {
			delete(next.assetReferences, id)
			delete(next.byAsset, id)
		}
	}
	delete(next.productAssets, productID)
	for route, item := range next.history {
		if item.ProductID == productID {
			delete(next.history, route)
		}
	}
	for _, item := range history {
		if item.State == "redirect" || item.State == "gone" {
			next.history[item.Route] = item
		}
	}
	if found {
		for _, suffix := range []string{"", ".json", ".md"} {
			if _, conflict := next.byRoute[activation.Route+suffix]; conflict {
				return fail(errors.New("active publication route conflicts"))
			}
		}
		next.byID[productID] = activation
		next.byRoute[activation.Route] = routeTarget{publication: activation, file: "index.html", contentType: "text/html; charset=utf-8"}
		next.byRoute[activation.Route+".json"] = routeTarget{publication: activation, file: "product.json", contentType: "application/json; charset=utf-8"}
		next.byRoute[activation.Route+".md"] = routeTarget{publication: activation, file: "product.md", contentType: "text/markdown; charset=utf-8"}
		for _, asset := range assets {
			next.byAsset[asset.asset.ID] = asset
			next.assetReferences[asset.asset.ID]++
			next.productAssets[productID] = append(next.productAssets[productID], asset.asset.ID)
		}
	}
	next.views = make([]PublicView, 0, len(next.byID))
	for _, view := range e.index.views {
		if view.ID != productID {
			next.views = append(next.views, view)
		}
	}
	if found {
		view := activation.View
		at := sort.Search(len(next.views), func(i int) bool {
			return next.views[i].PartNumber > view.PartNumber || next.views[i].PartNumber == view.PartNumber && next.views[i].ID >= view.ID
		})
		next.views = append(next.views, PublicView{})
		copy(next.views[at+1:], next.views[at:])
		next.views[at] = view
	}
	// An old aggregate can contain revoked text, counts, or old routes. Deny it
	// immediately; a bounded worker batch pre-generates its replacement outside
	// the admission gate, and installs it only against this exact snapshot.
	next.machine = nil
	next.specInventory = maps.Clone(next.specInventory)
	delete(next.specInventory, productID)
	if found {
		next.specInventory[productID] = SpecCandidateBinding{ProductID: activation.ProductID, PublicRevision: activation.PublicRevision, SourceRevision: activation.SourceRevision, SiteEpoch: activation.SiteEpoch, VisibilityGeneration: activation.VisibilityGeneration, ProjectionVersion: SpecProjectionVersion, SpecIDs: SpecProjectionIDs(activation.View)}
	}
	// The canonical adapter digest serializes every Product's numeric facts.
	// Compute it alongside the bounded derived refresh, never per activation.
	next.specDigest = ""
	next.specDigestError = nil
	e.index = &next
	e.specDirty.Store(true)
	e.Wake()
	return nil
}

func isMachinePath(path string) bool {
	return path == "/sitemap.xml" || path == "/robots.txt" || path == "/llms.txt" || path == CatalogManifestPath || strings.HasPrefix(path, "/catalog/products-") && strings.HasSuffix(path, ".json")
}

func (e *Engine) trackPublicationDependencies(productID string, revision int64) {
	e.gate.Lock()
	defer e.gate.Unlock()
	if e.pendingDependencies == nil {
		e.pendingDependencies = make(map[string]int64)
	}
	if revision > e.pendingDependencies[productID] {
		e.pendingDependencies[productID] = revision
	}
}

func (e *Engine) refreshMachine(ctx context.Context) error {
	e.machineRefreshMu.Lock()
	defer e.machineRefreshMu.Unlock()
	e.gate.RLock()
	snapshot := e.index
	if snapshot == nil && len(e.pendingDependencies) == 0 {
		e.gate.RUnlock()
		return nil // No visibility installation occurred (including drain).
	}
	if snapshot == nil || snapshot.unhealthy != nil {
		e.gate.RUnlock()
		return ErrPublicUnavailable
	}
	pending := maps.Clone(e.pendingDependencies)
	e.gate.RUnlock()
	if snapshot.machine == nil {
		machine, err := MachineRepresentations(snapshot.views, snapshot.site, e.baseURL, snapshot.siteEpoch)
		if err != nil {
			return err
		}
		e.gate.Lock()
		if e.index != snapshot {
			e.gate.Unlock()
			return nil // A newer install remains dirty for the next bounded pass.
		}
		next := *snapshot
		next.machine = machine
		e.index = &next
		e.gate.Unlock()
	}
	for id, revision := range pending {
		if err := e.repository.ConvergePublicationDependencies(ctx, id, revision); err != nil {
			return err
		}
		e.gate.Lock()
		if e.pendingDependencies[id] == revision {
			delete(e.pendingDependencies, id)
		}
		e.gate.Unlock()
	}
	return nil
}
