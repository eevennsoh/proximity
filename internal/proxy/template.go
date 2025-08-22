package proxy

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Template struct {
	logger *log.Logger

	// Storage which lasts for the lifetime of the proxy.
	permanentStorage map[string]string
}

func newTemplate(logger *log.Logger) *Template {
	t := &Template{
		logger:           logger,
		permanentStorage: make(map[string]string),
	}

	return t
}

func (t *Template) functionsWithStorage(temporaryStorage map[string]string) template.FuncMap {
	return template.FuncMap{
		"toJson": func(v any) string {
			b, err := json.Marshal(v)
			if err != nil {
				t.logger.Println(err)
				return ""
			}

			return string(b)
		},
		"getType": func(v any) string {
			return reflect.TypeOf(v).Kind().String()
		},
		"safeEncode": func(v any) string {
			jsonBytes, err := json.Marshal(v)
			if err != nil {
				t.logger.Println(err)
				return ""
			}

			safeString := string(jsonBytes)
			safeString, _ = strings.CutPrefix(safeString, "\"")
			safeString, _ = strings.CutSuffix(safeString, "\"")
			return safeString
		},
		"trim": func(str, prefix, suffix string) string {
			str = strings.TrimPrefix(str, prefix)
			str = strings.TrimSuffix(str, suffix)
			return prefix + str + suffix
		},
		"timestamp": func() string {
			return fmt.Sprintf("%d", time.Now().Unix())
		},
		"set": func(key string, val any) string {
			temporaryStorage[key] = fmt.Sprintf("%v", val)
			return ""
		},
		"get": func(key string) string {
			return temporaryStorage[key]
		},
		"sum": func(nums ...string) string {
			total := 0

			for _, numStr := range nums {
				num, err := strconv.Atoi(numStr)
				if err != nil {
					t.logger.Println(err)
					return ""
				}

				total += num
			}

			return fmt.Sprintf("%d", total)
		},
		"slauthtoken": func(groups string, audience string, environment string) string {
			token, exists := t.permanentStorage["token"]

			// If there is an existing token and it is still valid then use it.
			if exists && !t.tokenHasExpired(token) {
				t.logger.Println("use existing token")
				return token
			}

			t.logger.Println("requesting slauth token")

			token, err := t.requestSlauthToken(strings.Split(groups, ","), audience, environment)
			if err != nil {
				t.logger.Println(err)
				return ""
			}

			t.permanentStorage["token"] = token
			return token
		},
	}
}

func (t *Template) requestSlauthToken(groups []string, audience string, environment string) (string, error) {
	// Build arguments for: atlas slauth token -g <groups> --aud <audience> -e <environment>
	args := []string{"slauth", "token"}

	if len(groups) > 0 {
		args = append(args, "-g", strings.Join(groups, " "))
	}

	if audience != "" {
		args = append(args, "--aud", audience)
	}

	if environment != "" {
		args = append(args, "-e", environment)
	}

	cmd := exec.Command("/opt/atlassian/bin/atlas", args...)

	// Capture stdout (token) and return it trimmed
	out, err := cmd.Output()

	if err != nil {
		// Include stderr if available for easier troubleshooting
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("atlas slauth token failed: %v: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}

		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func (t *Template) tokenHasExpired(token string) bool {
	// Parse without verifying signature to read claims only
	parser := jwt.NewParser()

	parsed, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		t.logger.Println("tokenHasExpired: jwt parse error:", err)
		return true
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return true
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return true
	}

	// Consider tokens expiring within the next 30 seconds as expired
	return time.Until(exp.Time) <= 30*time.Second
}
