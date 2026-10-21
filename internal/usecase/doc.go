// Package usecase holds the application's business logic.
//
// A use case depends only on domain types and on the repository ports it needs; it
// never imports a concrete adapter. Every method takes a context.Context first and
// returns errors wrapped around the domain sentinels so the delivery layer can map
// them onto HTTP status codes.
package usecase
