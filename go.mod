module github.com/zoobzio/flux

go 1.23.0

toolchain go1.24.5

require (
	github.com/zoobzio/pipz v0.0.0-00010101000000-000000000000
	github.com/zoobzio/streamz v0.0.0-00010101000000-000000000000
)

require golang.org/x/time v0.12.0 // indirect

replace github.com/zoobzio/pipz => ../pipz

replace github.com/zoobzio/streamz => ../streamz
