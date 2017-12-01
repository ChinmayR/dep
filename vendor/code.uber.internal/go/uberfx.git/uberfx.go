// Package uberfx bundles all the dependencies required to build an Uber
// service except an RPC server (which is provided by yarpcfx).
package uberfx

import (
	configfx "code.uber.internal/go/configfx.git"
	debugfx "code.uber.internal/go/debugfx.git"
	envfx "code.uber.internal/go/envfx.git"
	galileofx "code.uber.internal/go/galileofx.git"
	healthfx "code.uber.internal/go/healthfx.git"
	jaegerfx "code.uber.internal/go/jaegerfx.git"
	maxprocsfx "code.uber.internal/go/maxprocsfx.git"
	runtimefx "code.uber.internal/go/runtimefx.git"
	sentryfx "code.uber.internal/go/sentryfx.git"
	servicefx "code.uber.internal/go/servicefx.git"
	systemportfx "code.uber.internal/go/systemportfx.git"
	tallyfx "code.uber.internal/go/tallyfx.git"
	versionfx "code.uber.internal/go/versionfx.git"
	zapfx "code.uber.internal/go/zapfx.git"
	"go.uber.org/fx"
)

// Version is the current package version.
const Version = "1.1.0"

// Module is a single fx.Option that provides all the common objects required
// to bootstrap an Uber service, including configuration, logging, metrics,
// and tracing. It doesn't depend on go-common.
//
// Since fx only instantiates types that your application uses, there's
// virtually no performance penalty if you only use a subset of the provided
// objects.
//
// The only configuration required for the stack is a service name and owner.
// In YAML, this configuration is sufficient (in both production and local
// development):
//
//  service:
//    name: your-service-name
var Module = fx.Options(
	configfx.Module,
	debugfx.Module,
	envfx.Module,
	galileofx.Module,
	healthfx.Module,
	maxprocsfx.Module,
	jaegerfx.Module,
	runtimefx.Module,
	sentryfx.Module,
	servicefx.Module,
	systemportfx.Module,
	tallyfx.Module,
	versionfx.Module,
	zapfx.Module,

	fx.Invoke(reportVersion),
)

// Since we provide this type and this function isn't exported, there's no
// need for the usual Params/Result pattern.
func reportVersion(version *versionfx.Reporter) error {
	return version.Report("uberfx", Version)
}
