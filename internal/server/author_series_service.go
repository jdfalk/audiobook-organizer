// file: internal/server/author_series_service.go
// version: 1.1.0
// guid: f6a7b8c9-d0e1-2f3a-4b5c-6d7e8f9a0b1c

package server

import (
	"github.com/jdfalk/audiobook-organizer/internal/database"
)

type AuthorSeriesService struct {
	db database.Store
}

func NewAuthorSeriesService(db database.Store) *AuthorSeriesService {
	return &AuthorSeriesService{db: db}
}

type AuthorListResponse struct {
	Items []database.Author `json:"items"`
	Count int               `json:"count"`
}

type SeriesListResponse struct {
	Items []database.Series `json:"items"`
	Count int               `json:"count"`
}

func (as *AuthorSeriesService) ListAuthors() (*AuthorListResponse, error) {
	authors, err := as.db.GetAllAuthors()
	if err != nil {
		return nil, err
	}
	if authors == nil {
		authors = []database.Author{}
	}
	return &AuthorListResponse{
		Items: authors,
		Count: len(authors),
	}, nil
}

func (as *AuthorSeriesService) ListSeries() (*SeriesListResponse, error) {
	series, err := as.db.GetAllSeries()
	if err != nil {
		return nil, err
	}
	if series == nil {
		series = []database.Series{}
	}
	return &SeriesListResponse{
		Items: series,
		Count: len(series),
	}, nil
}
