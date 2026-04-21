package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/igor/shelfy/internal/domain"
	"github.com/igor/shelfy/internal/observability"
	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListVisibleProducts(ctx context.Context, userID int64, mode string, now time.Time) ([]domain.Product, error) {
	switch mode {
	case "soon":
		items, err := s.queries.ListSoonProducts(ctx, sqlcgen.ListSoonProductsParams{
			UserID:  userID,
			Column2: pgDateFromTime(now),
			Column3: pgDateFromTime(now.AddDate(0, 0, 3)),
		})
		if err != nil {
			return nil, err
		}
		return productsFromModels(items), nil
	case "expired":
		items, err := s.queries.ListExpiredProducts(ctx, sqlcgen.ListExpiredProductsParams{
			UserID:  userID,
			Column2: pgDateFromTime(now),
		})
		if err != nil {
			return nil, err
		}
		return productsFromModels(items), nil
	default:
		items, err := s.queries.ListActiveProducts(ctx, userID)
		if err != nil {
			return nil, err
		}
		return productsFromModels(items), nil
	}
}

func (s *Store) ListVisibleProductsPage(ctx context.Context, userID int64, mode string, now time.Time, limit, offset int) ([]domain.Product, int, error) {
	if limit <= 0 {
		limit = 1
	}
	if offset < 0 {
		offset = 0
	}
	switch mode {
	case "soon":
		totalCount, err := s.queries.CountSoonProducts(ctx, sqlcgen.CountSoonProductsParams{
			UserID:  userID,
			Column2: pgDateFromTime(now),
			Column3: pgDateFromTime(now.AddDate(0, 0, 3)),
		})
		if err != nil {
			return nil, 0, err
		}
		items, err := s.queries.ListSoonProductsPage(ctx, sqlcgen.ListSoonProductsPageParams{
			UserID:  userID,
			Column2: pgDateFromTime(now),
			Column3: pgDateFromTime(now.AddDate(0, 0, 3)),
			Limit:   int32(limit),
			Offset:  int32(offset),
		})
		if err != nil {
			return nil, 0, err
		}
		return productsFromModels(items), int(totalCount), nil
	case "expired":
		items, err := s.queries.ListExpiredProducts(ctx, sqlcgen.ListExpiredProductsParams{
			UserID:  userID,
			Column2: pgDateFromTime(now),
		})
		if err != nil {
			return nil, 0, err
		}
		return productsFromModels(items), len(items), nil
	default:
		totalCount, err := s.queries.CountActiveProducts(ctx, userID)
		if err != nil {
			return nil, 0, err
		}
		items, err := s.queries.ListActiveProductsPage(ctx, sqlcgen.ListActiveProductsPageParams{
			UserID: userID,
			Limit:  int32(limit),
			Offset: int32(offset),
		})
		if err != nil {
			return nil, 0, err
		}
		return productsFromModels(items), int(totalCount), nil
	}
}

func (s *Store) GetProduct(ctx context.Context, productID int64) (domain.Product, error) {
	item, err := s.queries.GetProduct(ctx, productID)
	if err != nil {
		return domain.Product{}, err
	}
	return productFromModel(item), nil
}

func (s *Store) CreateProductFromDraft(ctx context.Context, draftID int64) (domain.Product, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Product{}, err
	}
	defer tx.Rollback(ctx)

	qtx := s.queries.WithTx(tx)
	draftModel, err := qtx.LockDraftForConfirmation(ctx, draftID)
	if err != nil {
		return domain.Product{}, err
	}
	draft, err := draftFromModel(draftModel)
	if err != nil {
		return domain.Product{}, err
	}
	if draft.Status == domain.DraftStatusConfirmed {
		if draft.ConfirmedProductID == nil {
			return domain.Product{}, fmt.Errorf("draft %d is confirmed but has no product reference", draftID)
		}
		productModel, err := qtx.GetProductByID(ctx, *draft.ConfirmedProductID)
		if err != nil {
			return domain.Product{}, err
		}
		product := productFromModel(productModel)
		if err := tx.Commit(ctx); err != nil {
			return domain.Product{}, err
		}
		s.logger.InfoContext(ctx, "product_reused_from_confirmed_draft", observability.LogAttrs(ctx,
			"draft_id", draftID,
			"product_id", product.ID,
		)...)
		return product, nil
	}
	switch draft.Status {
	case domain.DraftStatusReady, domain.DraftStatusEditingName, domain.DraftStatusEditingDate:
	default:
		return domain.Product{}, fmt.Errorf("draft %d is not confirmable from status %s", draftID, draft.Status)
	}
	if draft.DraftExpiresOn == nil || strings.TrimSpace(draft.DraftName) == "" {
		return domain.Product{}, fmt.Errorf("draft %d is incomplete", draftID)
	}
	productModel, err := qtx.CreateProduct(ctx, sqlcgen.CreateProductParams{
		UserID:            draft.UserID,
		Name:              draft.DraftName,
		NormalizedName:    normalizeName(draft.DraftName),
		ExpiresOn:         pgDateFromTime(*draft.DraftExpiresOn),
		RawDeadlinePhrase: emptyToNil(draft.RawDeadlinePhrase),
		SourceKind:        string(draft.SourceKind),
	})
	if err != nil {
		return domain.Product{}, err
	}
	product := productFromModel(productModel)
	if err := qtx.MarkDraftConfirmed(ctx, sqlcgen.MarkDraftConfirmedParams{
		ID:                 draftID,
		Status:             string(domain.DraftStatusConfirmed),
		ConfirmedProductID: &product.ID,
	}); err != nil {
		return domain.Product{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Product{}, err
	}
	s.logger.InfoContext(ctx, "product_created_from_draft", observability.LogAttrs(ctx,
		"draft_id", draftID,
		"product_id", product.ID,
		"product_name", product.Name,
		"expires_on", product.ExpiresOn.Format("2006-01-02"),
	)...)
	return product, nil
}

func (s *Store) UpdateProductStatus(ctx context.Context, productID int64, status domain.ProductStatus) error {
	err := s.queries.UpdateProductStatus(ctx, sqlcgen.UpdateProductStatusParams{
		ID:     productID,
		Status: string(status),
	})
	if err == nil {
		s.logger.InfoContext(ctx, "product_status_updated", observability.LogAttrs(ctx, "product_id", productID, "status", status)...)
	}
	return err
}

func (s *Store) ActiveProductsExist(ctx context.Context, productIDs []int64) (bool, error) {
	if len(productIDs) == 0 {
		return false, nil
	}
	return s.queries.ActiveProductsExist(ctx, productIDs)
}
