package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"prods/internal/catalog"
)

var ErrInvalidProductPage = errors.New("invalid product page")

type ProductPageQuery struct {
	Page, PageSize  int
	IncludeArchived bool
	Sort, Order     string
}

type ProductPage struct {
	Data  []catalog.Product `json:"data"`
	Total int               `json:"total"`
}

// PageProducts reads the count and rows from one snapshot. SQL identifiers come
// only from this allowlist; user values never become SQL fragments.
func (s *Store) PageProducts(ctx context.Context, query ProductPageQuery) (ProductPage, error) {
	result := ProductPage{Data: []catalog.Product{}}
	if query.Page < 1 || query.Page > 1_000_000 || query.PageSize < 1 || query.PageSize > 100 {
		return result, ErrInvalidProductPage
	}
	sort := map[string]string{"updated_at": "updated_at", "part_number": "part_number", "name": "name"}[query.Sort]
	if sort == "" || (query.Order != "asc" && query.Order != "desc") {
		return result, ErrInvalidProductPage
	}
	where := "record_state='current'"
	if query.IncludeArchived {
		where = "1=1"
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM products WHERE `+where).Scan(&result.Total); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+productColumns+` FROM products WHERE `+where+
		` ORDER BY `+sort+` `+query.Order+`,id ASC LIMIT ? OFFSET ?`, query.PageSize, (query.Page-1)*query.PageSize)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		product, err := scanProduct(rows)
		if err != nil {
			return result, err
		}
		result.Data = append(result.Data, product)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
