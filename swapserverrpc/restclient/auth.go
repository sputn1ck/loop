package restclient

import (
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// L402Challenge is a parsed L402 or legacy LSAT WWW-Authenticate challenge.
type L402Challenge struct {
	Scheme   string
	Macaroon string
	Invoice  string
	Params   map[string]string
	Raw      string
}

// PaymentRequiredError reports an L402 challenge that was not satisfied.
// Challenge secrets are deliberately omitted from Error.
type PaymentRequiredError struct {
	StatusCode int
	Challenge  L402Challenge
}

// Error returns a non-secret description of the challenge.
func (e *PaymentRequiredError) Error() string {
	return fmt.Sprintf("Loop server authorization payment required (HTTP %d)",
		e.StatusCode)
}

// GRPCStatus makes status.Code classify an unsatisfied challenge as an
// authentication failure.
func (e *PaymentRequiredError) GRPCStatus() *status.Status {
	return status.New(codes.Unauthenticated, e.Error())
}

func responseL402Challenge(resp *http.Response) (L402Challenge, bool) {
	if resp.StatusCode != http.StatusPaymentRequired &&
		resp.StatusCode != http.StatusUnauthorized {

		return L402Challenge{}, false
	}

	for _, header := range resp.Header.Values("WWW-Authenticate") {
		if challenge, ok := ParseL402Challenge(header); ok {
			return challenge, true
		}
	}

	if resp.StatusCode == http.StatusPaymentRequired {
		return L402Challenge{}, true
	}

	return L402Challenge{}, false
}

// ParseL402Challenge parses an L402 or legacy LSAT authentication challenge.
func ParseL402Challenge(header string) (L402Challenge, bool) {
	raw := strings.TrimSpace(header)
	separator := strings.IndexAny(raw, " \t")
	if separator <= 0 {
		return L402Challenge{}, false
	}

	scheme := raw[:separator]
	if !strings.EqualFold(scheme, "L402") &&
		!strings.EqualFold(scheme, "LSAT") {

		return L402Challenge{}, false
	}

	params := parseAuthParams(raw[separator+1:])
	return L402Challenge{
		Scheme:   scheme,
		Macaroon: params["macaroon"],
		Invoice:  params["invoice"],
		Params:   params,
		Raw:      raw,
	}, true
}

func parseAuthParams(value string) map[string]string {
	params := make(map[string]string)
	for offset := 0; offset < len(value); {
		for offset < len(value) &&
			(value[offset] == ',' || value[offset] == ' ' ||
				value[offset] == '\t') {

			offset++
		}
		if offset >= len(value) {
			break
		}

		keyStart := offset
		for offset < len(value) && value[offset] != '=' &&
			value[offset] != ',' {

			offset++
		}
		if offset >= len(value) || value[offset] != '=' {
			for offset < len(value) && value[offset] != ',' {
				offset++
			}
			continue
		}

		key := strings.ToLower(strings.TrimSpace(value[keyStart:offset]))
		offset++
		if key == "" {
			continue
		}

		var parameter string
		if offset < len(value) && value[offset] == '"' {
			offset++
			var builder strings.Builder
			for offset < len(value) {
				switch value[offset] {
				case '\\':
					offset++
					if offset < len(value) {
						builder.WriteByte(value[offset])
						offset++
					}

				case '"':
					offset++
					parameter = builder.String()
					goto parsed

				default:
					builder.WriteByte(value[offset])
					offset++
				}
			}
			parameter = builder.String()
		} else {
			parameterStart := offset
			for offset < len(value) && value[offset] != ',' {
				offset++
			}
			parameter = strings.TrimSpace(value[parameterStart:offset])
		}

	parsed:
		params[key] = parameter
		for offset < len(value) && value[offset] != ',' {
			offset++
		}
	}

	return params
}
