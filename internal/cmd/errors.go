package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/dotcommander/yai/internal/errs"
	"github.com/dotcommander/yai/internal/present"
)

func handleError(err error) {
	maybeWriteMemProfile()

	// exhaust stdin
	if !present.IsInputTTY() {
		_, _ = io.ReadAll(os.Stdin)
	}

	format := "\n%s\n\n"

	var ferr flagParseError
	if errors.As(err, &ferr) {
		args := []any{
			fmt.Sprintf(
				"Check out %s %s",
				present.StderrStyles().InlineCode.Render("yai -h"),
				present.StderrStyles().Comment.Render("for help."),
			),
			fmt.Sprintf(
				ferr.ReasonFormat(),
				present.StderrStyles().InlineCode.Render(ferr.Flag()),
			),
		}
		fmt.Fprintf(os.Stderr, format+"%s\n\n", args...)
		return
	}

	var merr errs.Error
	if errors.As(err, &merr) {
		formatArgs := []any{present.StderrStyles().ErrPadding.Render(present.StderrStyles().ErrorHeader.String(), merr.Reason)}
		if merr.Err != huh.ErrUserAborted {
			format += "%s\n\n"
			formatArgs = append(formatArgs, present.StderrStyles().ErrPadding.Render(present.StderrStyles().ErrorDetails.Render(err.Error())))
		}
		fmt.Fprintf(os.Stderr, format, formatArgs...)
		return
	}

	fmt.Fprintf(os.Stderr, format, present.StderrStyles().ErrPadding.Render(present.StderrStyles().ErrorDetails.Render(err.Error())))
}
