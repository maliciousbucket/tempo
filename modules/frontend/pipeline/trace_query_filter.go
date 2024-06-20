package pipeline

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
)

type traceQueryFilterWare struct {
	next    http.RoundTripper
	filters []*regexp.Regexp
	parseFn func(r *http.Request) string
}

func NewTraceQueryFilterWare(next http.RoundTripper) http.RoundTripper {
	return &traceQueryFilterWare{
		next: next,
	}
}

func NewTraceQueryFilterWareWithDenyList(denyList []string, parseFunc func(r *http.Request) string) Middleware {
	filter := make([]*regexp.Regexp, len(denyList)+1)
	for i := range denyList {
		exp, err := regexp.Compile(denyList[i])
		if err == nil {
			filter[i] = exp
		}
	}

	return MiddlewareFunc(func(next http.RoundTripper) http.RoundTripper {
		return traceQueryFilterWare{
			next:    next,
			filters: filter,
			parseFn: parseFunc,
		}
	})
}

func (c traceQueryFilterWare) RoundTrip(req *http.Request) (*http.Response, error) {
	if c.filters == nil || len(c.filters) == 0 {
		return c.next.RoundTrip(req)
	}

	query := c.parseFn(req)

	if len(query) == 0 {
		return c.next.RoundTrip(req)
	}

	match := make(chan bool, len(c.filters))
	wg := sync.WaitGroup{}
	for range c.filters {
		wg.Add(1)
	}

	go func(qry string) {
		defer wg.Done()
		for _, re := range c.filters {
			if re.MatchString(qry) {
				match <- true
				return
			}
		}
		match <- false
	}(query)

	go func() {
		wg.Wait()
		close(match)
	}()

	if <-match {

		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Status:     http.StatusText(http.StatusBadRequest),
			Body:       io.NopCloser(strings.NewReader("Query is temporarily blocked by your administrator.")),
		}, nil

	}
	return c.next.RoundTrip(req)
}
