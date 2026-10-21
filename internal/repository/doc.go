// Package repository declares the data-access ports consumed by the usecase layer.
//
// Implementations live in subpackages such as repository/postgres. An implementation
// translates driver-level failures into domain errors, so no layer above ever has to
// know about pgx.ErrNoRows or a connection error.
package repository
