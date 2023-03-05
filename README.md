# chi-http-hclog
An attempt for a logger middleware for go-chi using hclog (based on [httplog](https://github.com/go-chi/httplog))

### Example
```go
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Vahatra/chi-http-hclog"
)

func main() {
	r := chi.NewRouter()
	l := httplog.NewLogger(httplog.Options{
		Name:       "HelloService",
		JSONFormat: true,
		Concise:    true,
		Tags: map[string]string{
			"version": "v1.0-81aa4244d9fc8076a",
			"env":     "dev",
		},
	})
	r.Use(httplog.RequestLogger(l))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello World!"))
	})
	http.ListenAndServe(":3000", r)
}
```