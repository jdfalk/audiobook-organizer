// file: internal/database/iface_series.go
// version: 1.0.0
// guid: 459a6734-95fb-437c-bb97-6baecc64aba4

package database

// SeriesReader is the read-only series slice.
type SeriesReader interface {
	GetAllSeries() ([]Series, error)
	GetSeriesByID(id int) (*Series, error)
	GetSeriesByName(name string, authorID *int) (*Series, error)
	GetAllSeriesBookCounts() (map[int]int, error)
	GetAllSeriesFileCounts() (map[int]int, error)
}

// SeriesWriter is the write-only series slice.
type SeriesWriter interface {
	CreateSeries(name string, authorID *int) (*Series, error)
	DeleteSeries(id int) error
	UpdateSeriesName(id int, name string) error
}

// SeriesStore combines both halves.
type SeriesStore interface {
	SeriesReader
	SeriesWriter
}
