// whole-file feature:metrics
package routes

import (
	bytes "bytes"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	metrics "{{MODULE_PATH}}/server/metrics"
)

// RequireMetrics makes /metrics answer 404 while metrics.enabled or
// auth.api_keys is off, like RequireMCP.
func ( handlers *Handlers ) RequireMetrics( c fiber.Ctx ) ( err error ) {
	cfg := handlers.cfg()
	if cfg.MetricsEnabled == false || cfg.APIKeysEnabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

// getMetrics is what a Prometheus scrape reads. See server/metrics.
//
// @route("GET /metrics", summary="Prometheus metrics",
//     tag="Monitoring", access=KEY_ONLY, role=ADMIN, feature="metrics", reply_type="text/plain",
//     mcp_http="Scraped by Prometheus over HTTP.",
// )
func ( handlers *Handlers ) getMetrics( c fiber.Ctx , in *apidoc.None ) ( err error ) {
	extra := metrics.Extra{}
	// >>> feature:logs
	if handlers.Logs.Enabled() {
		extra.LogsDropped = handlers.Logs.Dropped()
	}
	// <<< feature:logs
	if handlers.Jobs != nil {
		for _ , job := range handlers.Jobs.Status() {
			extra.Jobs = append( extra.Jobs , metrics.Job{ Name: job.Name , LastSuccess: job.LastSuccess , LastFailure: job.LastFailure } )
		}
	}
	var out bytes.Buffer
	handlers.Metrics.Write( &out , extra )
	c.Set( fiber.HeaderContentType , "text/plain; version=0.0.4; charset=utf-8" )
	err = c.Send( out.Bytes() )
	return
}
