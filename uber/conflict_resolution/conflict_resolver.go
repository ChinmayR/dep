package conflict_resolution

import (
	"fmt"
	"log"
	"strings"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"
	"github.com/golang/dep/internal/importers/base"
	"github.com/pkg/errors"
)

type Mode int

const (
	MIGRATE_MODE Mode = iota
	NORMAL_MODE
)

func HandleSolveConflicts(ctx *dep.Ctx, manifest *dep.Manifest, operatingMode Mode, err error) error {
	ovrPkgs, errInternal := gps.HandleErrors(ctx.Out, err)
	if errInternal != nil {
		ctx.Err.Println(errInternal)
		return errInternal
	}
	if len(ovrPkgs) == 0 {
		return errors.New("No resolution options to provide")
	}

	ovrPkgSelected, errInternal := getUserSelection(ctx, ovrPkgs)
	if errInternal != nil {
		ctx.Err.Println(errInternal)
		return errInternal
	}

	if operatingMode == MIGRATE_MODE {
		// append to the custom config when operating in the migration mode since the manifest does not exist yet
		// as the repo is not on dep yet
		errInternal = base.AddOverrideToConfig(ovrPkgSelected.Name, ovrPkgSelected.Constraint.String(), ovrPkgSelected.Source,
			ctx.WorkingDir, ctx.Out)
	} else if operatingMode == NORMAL_MODE {
		// append the override to the manifest since the repo is already on dep
		manifest.Ovr[gps.ProjectRoot(ovrPkgSelected.Name)] = gps.ProjectProperties{
			Source:     ovrPkgSelected.Source,
			Constraint: ovrPkgSelected.Constraint,
		}
	} else {
		return errors.New("unsupported operating mode")
	}
	return errInternal
}

func getUserSelection(ctx *dep.Ctx, ovrPkgOptions []gps.OverridePackage) (*gps.OverridePackage, error) {
	ctx.Out.Print("Select an option: ")
	var i int
	fmt.Scan(&i)
	var ovrPkgSelected *gps.OverridePackage
	if i == gps.EXIT_NUM {
		ctx.Out.Println("User selected exit")
		return nil, errors.New("User selected exit")
	} else if i == gps.CUSTOM_NUM { //provide an option to set a custom override not in the recommendation list
		// get override root name
		// TODO: add validation
		overName := getUserInput(ctx.Out, "Package Name: ")

		// get override type (, none(*), branch, revision, semver)
		// TODO: add validation
		overType := getUserInput(ctx.Out, "Override type (none, branch, revision, semver): ")

		var constraint string
		if !strings.EqualFold(overType, "none") {
			// get override for the type captured above
			// TODO: add validation
			constraint = getUserInput(ctx.Out, "Override: ")
		}

		// get source override
		// TODO: add validation
		overSource := getUserInput(ctx.Out, "Override source: ")

		ovrPkgParsed, err := parseUserInputOverride(overName, overType, constraint, overSource)
		if err != nil {
			return nil, err
		}
		ovrPkgSelected = ovrPkgParsed
	} else {
		ovrPkgSelected = &ovrPkgOptions[i-2]
	}
	return ovrPkgSelected, nil
}

func getUserInput(logger *log.Logger, inputDescription string) string {
	logger.Print(inputDescription)
	var input string
	fmt.Scanln(&input)
	return strings.Trim(input, " ")
}

func parseUserInputOverride(overName, overType, constraint, overSource string) (*gps.OverridePackage, error) {
	var parsedConstraint gps.Constraint
	switch overType {
	case "none":
		parsedConstraint = gps.Any()
	case "branch":
		parsedConstraint = gps.NewBranch(constraint)
	case "revision":
		parsedConstraint = gps.Revision(constraint)
	case "semver":
		semverIC, err := gps.NewSemverConstraint(constraint)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create semver constraint")
		}
		parsedConstraint = semverIC
	default:
		return nil, errors.New("received unrecognized override type, not of type (any, branch, revision, version")
	}
	return &gps.OverridePackage{
		Name:       overName,
		Constraint: parsedConstraint,
		Source:     overSource,
	}, nil
}
