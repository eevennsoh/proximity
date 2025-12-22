package template

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"text/template"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

type Renderer struct {
	logger *log.Logger
	mu     sync.RWMutex

	// Storage which lasts for the lifetime of the proxy.
	permanentStorage map[string]string
}

func NewRenderer(logger *log.Logger) *Renderer {
	return &Renderer{
		logger:           logger,
		permanentStorage: make(map[string]string),
	}
}

// Render renders content using either Expr or Go template, based on which is provided.
// If both are provided, Expr takes priority.
// Returns nil if neither is provided.
func (r *Renderer) Render(templateStr, exprStr string, input map[string]any, storage map[string]string) ([]byte, error) {
	// Expr takes priority if both are provided
	if strings.TrimSpace(exprStr) != "" {
		return r.RenderExpr(exprStr, input, storage)
	}

	// Fall back to Go template
	if strings.TrimSpace(templateStr) != "" {
		return r.RenderTemplate(templateStr, input, storage)
	}

	// Neither provided - return nil (no rendering needed)
	return nil, nil
}

// RenderTemplate renders using Go text/template
func (r *Renderer) RenderTemplate(templateStr string, input map[string]any, storage map[string]string) ([]byte, error) {
	tmpl, err := template.New("body").Funcs(r.FunctionsWithStorage(storage)).Parse(templateStr)
	if err != nil {
		return nil, err
	}

	var buf strings.Builder

	if err := tmpl.Execute(&buf, input); err != nil {
		return nil, err
	}

	return []byte(buf.String()), nil
}

func (r *Renderer) FunctionsWithStorage(temporaryStorage map[string]string) template.FuncMap {
	return template.FuncMap{
		"toJson":             r.toJsonFn,
		"getType":            r.getTypeFn,
		"safeEncode":         r.safeEncodeFn,
		"normalize":          r.normalizeFn,
		"trim":               r.trimFn,
		"timestamp":          r.timestampFn,
		"formattedTimestamp": r.formattedTimestampFn,
		"set":                r.setFn(temporaryStorage),
		"get":                r.getFn(temporaryStorage),
		"sum":                r.sumFn,
		"subtract":           r.subtractFn,
		"regexFind":          r.regexFindFn,
		"slauthtoken":        r.slauthtokenFn,
	}
}

func (r *Renderer) requestSlauthToken(groups []string, audience string, environment string) (string, error) {
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

func (r *Renderer) tokenHasExpired(token string) bool {
	// Parse without verifying signature to read claims only
	parser := jwt.NewParser()

	parsed, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		r.logger.Println("tokenHasExpired: jwt parse error:", err)
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

func (r *Renderer) toJsonFn(v string) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	return string(b), nil
}

func (r *Renderer) getTypeFn(v any) string {
	return reflect.TypeOf(v).Kind().String()
}

func (r *Renderer) safeEncodeFn(v any) (string, error) {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return "", err
	}

	safeString := string(jsonBytes)

	safeString, _ = strings.CutPrefix(safeString, "\"")
	safeString, _ = strings.CutSuffix(safeString, "\"")
	return safeString, nil
}

func (r *Renderer) normalizeFn(str, prefix, suffix string) string {
	str = strings.TrimPrefix(str, prefix)
	str = strings.TrimSuffix(str, suffix)
	return prefix + str + suffix
}

func (r *Renderer) trimFn(str, prefix, suffix string) string {
	str = strings.TrimPrefix(str, prefix)
	str = strings.TrimSuffix(str, suffix)
	return str
}

func (r *Renderer) timestampFn() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

func (r *Renderer) formattedTimestampFn(layout string) string {
	return fmt.Sprint(time.Now().Format(layout))
}

func (r *Renderer) setFn(temporaryStorage map[string]string) func(key string, val any) string {
	return func(key string, val any) string {
		temporaryStorage[key] = fmt.Sprintf("%v", val)
		return ""
	}
}

func (r *Renderer) getFn(temporaryStorage map[string]string) func(key string) string {
	return func(key string) string {
		return temporaryStorage[key]
	}
}

func (r *Renderer) sumFn(nums ...int) string {
	total := 0

	for _, num := range nums {
		total += num
	}

	return fmt.Sprintf("%d", total)
}

func (r *Renderer) subtractFn(a, b int) int {
	return a - b
}

func (r *Renderer) regexFindFn(pattern, s string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	matches := re.FindStringSubmatch(s)

	// Only expects a single capture group
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "", nil
}

func (r *Renderer) slauthtokenFn(groups string, audience string, environment string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	token, exists := r.permanentStorage["token"]

	// If there is an existing token and it is still valid then use it.
	if exists && !r.tokenHasExpired(token) {
		r.logger.Println("use existing token")
		return token, nil
	}

	r.logger.Println("requesting slauth token")

	token, err := r.requestSlauthToken(strings.Split(groups, ","), audience, environment)
	if err != nil {
		return "", err
	}

	r.permanentStorage["token"] = token
	return token, nil
}
