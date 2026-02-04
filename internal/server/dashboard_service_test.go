// file: internal/server/dashboard_service_test.go
// version: 1.0.0
// guid: a0b1c2d3-e4f5-6a7b-8c9d-0e1f2a3b4c5d

package server

import (
	"testing"

	"github.com/jdfalk/audiobook-organizer/internal/database"
)

func TestDashboardService_CollectDashboardMetrics_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CountBooksFunc: func() (int, error) {
			return 42, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{
				{ID: 1, Name: "Author 1"},
				{ID: 2, Name: "Author 2"},
			}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{
				{ID: 1, Name: "Series 1"},
				{ID: 2, Name: "Series 2"},
				{ID: 3, Name: "Series 3"},
			}, nil
		},
	}

	service := NewDashboardService(mockDB)
	metrics, err := service.CollectDashboardMetrics()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if metrics.Books != 42 {
		t.Errorf("expected 42 books, got %d", metrics.Books)
	}
	if metrics.Authors != 2 {
		t.Errorf("expected 2 authors, got %d", metrics.Authors)
	}
	if metrics.Series != 3 {
		t.Errorf("expected 3 series, got %d", metrics.Series)
	}
}

func TestDashboardService_CollectDashboardMetrics_ZeroValues(t *testing.T) {
	mockDB := &database.MockStore{
		CountBooksFunc: func() (int, error) {
			return 0, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{}, nil
		},
	}

	service := NewDashboardService(mockDB)
	metrics, err := service.CollectDashboardMetrics()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if metrics == nil {
		t.Fatal("expected metrics, got nil")
	}
	if metrics.Books != 0 || metrics.Authors != 0 || metrics.Series != 0 {
		t.Error("expected all zero metrics")
	}
}

func TestDashboardService_GetHealthCheckResponse_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CountBooksFunc: func() (int, error) {
			return 10, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{}, nil
		},
	}

	service := NewDashboardService(mockDB)
	response := service.GetHealthCheckResponse("1.0.0")

	if response == nil {
		t.Fatal("expected response, got nil")
	}
	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", response.Status)
	}
	if response.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", response.Version)
	}
	if response.Metrics.Books != 10 {
		t.Errorf("expected 10 books in metrics, got %d", response.Metrics.Books)
	}
}

func TestDashboardService_CollectLibraryStats_Success(t *testing.T) {
	organized := "organized"
	imported := "imported"

	mockDB := &database.MockStore{
		CountBooksFunc: func() (int, error) {
			return 100, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{{ID: 1, Name: "Author"}}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{{ID: 1, Name: "Series"}}, nil
		},
		GetAllBooksFunc: func(limit, offset int) ([]database.Book, error) {
			return []database.Book{
				{ID: "1", LibraryState: &organized},
				{ID: "2", LibraryState: &imported},
			}, nil
		},
	}

	service := NewDashboardService(mockDB)
	stats, err := service.CollectLibraryStats()

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if stats.TotalBooks != 100 {
		t.Errorf("expected 100 total books, got %d", stats.TotalBooks)
	}
	if stats.OrganizedBooks != 1 {
		t.Errorf("expected 1 organized book, got %d", stats.OrganizedBooks)
	}
	if stats.UnorganizedBooks != 1 {
		t.Errorf("expected 1 unorganized book, got %d", stats.UnorganizedBooks)
	}
}

func TestDashboardService_CollectQuickMetrics_Success(t *testing.T) {
	mockDB := &database.MockStore{
		CountBooksFunc: func() (int, error) {
			return 50, nil
		},
		GetAllAuthorsFunc: func() ([]database.Author, error) {
			return []database.Author{
				{ID: 1, Name: "A"},
				{ID: 2, Name: "B"},
				{ID: 3, Name: "C"},
			}, nil
		},
		GetAllSeriesFunc: func() ([]database.Series, error) {
			return []database.Series{}, nil
		},
	}

	service := NewDashboardService(mockDB)
	metrics := service.CollectQuickMetrics()

	if metrics == nil {
		t.Fatal("expected metrics, got nil")
	}
	if metrics.BookCount != 50 {
		t.Errorf("expected 50 books, got %d", metrics.BookCount)
	}
	if metrics.AuthorCount != 3 {
		t.Errorf("expected 3 authors, got %d", metrics.AuthorCount)
	}
	if metrics.SeriesCount != 0 {
		t.Errorf("expected 0 series, got %d", metrics.SeriesCount)
	}
}
