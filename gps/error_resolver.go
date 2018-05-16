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
	Constraint string
}

const EXIT_NUM = 0

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

	var buf bytes.Buffer
	var optionStr []string
	nameStr := "%d) Set an override for %s"
	constraintStr := " on constraint %s"
	sourceStr := " for source %s"
	logOut.Printf("%d) Exit", EXIT_NUM)
	for i, ovrPkg := range ovrPkgs {
		if i > 5 {
			break
		}
		buf.Reset()
		fmt.Fprintf(&buf, nameStr, i+1, ovrPkg.Name)
		if ovrPkg.Constraint != "" {
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

	return ovrPkgs, nil
}
