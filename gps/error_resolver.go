package gps

import (
	"bytes"
	"fmt"
	"log"
)

type ErrorResolver interface {
	FixError(out *log.Logger) ([]OverridePackage, error)
}

type ErrorOptions interface {
	GetOptions() []OverridePackage
}

type OverridePackage struct {
	Name       string
	Source     string
	Constraint Constraint
}

const EXIT_NUM = 0
const CUSTOM_NUM = 1

func HandleErrors(logOut *log.Logger, err error) ([]OverridePackage, error) {
	logOut.Println(err)
	logOut.Println("========================")
	logOut.Println("   Resolution Options   ")
	logOut.Println("========================")

	var ovrPkgs []OverridePackage
	if te, ok := err.(ErrorResolver); ok {
		ovrPkgs, err = te.FixError(logOut)
	} else {
		logOut.Printf("Error resolution is not supported for type %T", err)
	}

	// there can be duplicate override options, so filter them out, for ex:
	// Set an override for blah.git on constraint ^1.0.0
	// Set an override for blah.git on constraint v1.0.0-rc3
	// Set an override for blah.git on constraint ^1.0.0
	// Set an override for blah.git on constraint v1.0.0-rc2
	var filteredOvrPkgs []OverridePackage
	for i, ovrPkg := range ovrPkgs {
		dupFound := false
		for j := 0; j < i; j++ {
			if ovrPkg.Name == ovrPkgs[j].Name &&
				ovrPkg.Constraint.String() == ovrPkgs[j].Constraint.String() &&
				ovrPkg.Source == ovrPkgs[j].Source {
				dupFound = true
			}
		}
		if !dupFound {
			filteredOvrPkgs = append(filteredOvrPkgs, ovrPkg)
		}
	}

	var buf bytes.Buffer
	var optionStr []string
	nameStr := "%d) Set an override for %s"
	constraintStr := " on constraint %s"
	sourceStr := " for source %s"
	logOut.Printf("%d) Exit", EXIT_NUM)
	logOut.Printf("%d) Custom override", CUSTOM_NUM)
	for i, ovrPkg := range filteredOvrPkgs {
		if i > 5 {
			break
		}
		buf.Reset()
		fmt.Fprintf(&buf, nameStr, i+2, ovrPkg.Name)
		if ovrPkg.Constraint.String() != "" {
			fmt.Fprintf(&buf, constraintStr, ovrPkg.Constraint)
		}
		if ovrPkg.Source != "" {
			fmt.Fprintf(&buf, sourceStr, ovrPkg.Source)
		}
		optionStr = append(optionStr, buf.String())
		if i == 0 {
			buf.WriteString(" (RECOMMENDED)")
		}
		logOut.Println(buf.String())
	}

	return filteredOvrPkgs, nil
}
