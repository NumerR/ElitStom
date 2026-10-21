// The web client is not Go. This file makes web/ a separate module so that "./..." from the
// repository root never descends into node_modules, where some packages ship Go sources.
module github.com/NumerR/ElitStom/web

go 1.22
