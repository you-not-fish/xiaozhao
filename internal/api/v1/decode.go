package v1

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

// validate is shared across handlers; go-playground/validator is safe for
// concurrent use after construction.
var validate = validator.New(validator.WithRequiredStructEnabled())

// decode reads a JSON body into dst and runs struct-tag validation. It
// returns typed errcode errors so handlers can forward them to the
// response writer without additional mapping.
func decode(r *http.Request, dst any) error {
	if r.Body == nil {
		return errcode.New(errcode.CodeInvalidArgument, "missing request body")
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return errcode.New(errcode.CodeInvalidArgument, "empty request body")
		}
		return errcode.Wrap(err, errcode.CodeInvalidArgument, "invalid json body")
	}
	if err := validate.Struct(dst); err != nil {
		return errcode.New(errcode.CodeInvalidArgument, formatValidationErr(err))
	}
	return nil
}

func formatValidationErr(err error) string {
	var verrs validator.ValidationErrors
	if ok := errorsAs(err, &verrs); !ok {
		return err.Error()
	}
	parts := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		parts = append(parts, fe.Field()+": "+fe.Tag())
	}
	return "validation failed: " + strings.Join(parts, ", ")
}

// errorsAs is a tiny wrapper to avoid importing "errors" solely for this test.
func errorsAs(err error, target any) bool {
	if err == nil {
		return false
	}
	tgt, ok := target.(*validator.ValidationErrors)
	if !ok {
		return false
	}
	v, ok := err.(validator.ValidationErrors)
	if !ok {
		return false
	}
	*tgt = v
	return true
}
