package internal

const (
	// CallerHeader is standard yarpc header indicating name of the service
	// sending the request.
	CallerHeader = "Rpc-Caller"

	// XUberSourceHeader Uber's previous standard http header for indicating
	// name of the service sending the request.
	// net/http takes care of tranforming to canonical capitalization.
	XUberSourceHeader = "X-Uber-Source"
)
