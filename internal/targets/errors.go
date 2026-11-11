package targets

import "fmt"

// ErrCaseNotFound reports that a requested switch-case / when-branch target
// could not be located in the enclosing function body.
func ErrCaseNotFound(caseTarget string) error {
	return fmt.Errorf("case target %q not found", caseTarget)
}
