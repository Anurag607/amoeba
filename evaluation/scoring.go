package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/anurgosw/agentic-moe/runtimekit"
)

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*[^\s,;]+`),
	regexp.MustCompile(`(?i)\bsk-[a-z0-9_-]{8,}`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func validateCases(cases []Case) error {
	if len(cases) == 0 {
		return fmt.Errorf("evaluation: at least one case is required")
	}
	seen := make(map[string]struct{}, len(cases))
	for _, item := range cases {
		if strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.Domain) == "" || strings.TrimSpace(item.Prompt) == "" {
			return fmt.Errorf("evaluation: every case requires id, domain, and prompt")
		}
		if _, exists := seen[item.ID]; exists {
			return fmt.Errorf("evaluation: duplicate case id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
		if len(item.Checks) == 0 {
			return fmt.Errorf("evaluation: case %q requires checks", item.ID)
		}
		for _, check := range item.Checks {
			if err := validateCheck(check); err != nil {
				return fmt.Errorf("evaluation: case %q: %w", item.ID, err)
			}
		}
	}
	return nil
}

func validateCheck(check Check) error {
	if strings.TrimSpace(check.Name) == "" {
		return fmt.Errorf("check name is required")
	}
	if check.Category != "" && !validFailureClass(check.Category) {
		return fmt.Errorf("check %q has unknown category %q", check.Name, check.Category)
	}
	switch check.Kind {
	case CheckContainsAll, CheckContainsAny, CheckExcludesAll, CheckExact, CheckJSONKeys:
		if len(check.Values) == 0 {
			return fmt.Errorf("check %q requires values", check.Name)
		}
	case CheckRegexp:
		if check.Pattern == "" {
			return fmt.Errorf("check %q requires a pattern", check.Name)
		}
		if _, err := regexp.Compile(check.Pattern); err != nil {
			return fmt.Errorf("check %q pattern: %w", check.Name, err)
		}
	case CheckMaxWords:
		if len(check.Values) != 1 {
			return fmt.Errorf("check %q requires one word limit", check.Name)
		}
		limit, err := strconv.Atoi(check.Values[0])
		if err != nil || limit <= 0 {
			return fmt.Errorf("check %q has invalid word limit", check.Name)
		}
	case CheckQuestion:
	default:
		return fmt.Errorf("check %q has unknown kind %q", check.Name, check.Kind)
	}
	return nil
}

func scoreAnswer(item Case, output string) (float64, []AssertionResult) {
	assertions := make([]AssertionResult, 0, len(item.Checks))
	passed := 0
	for _, check := range item.Checks {
		ok := checkAnswer(check, output)
		if ok {
			passed++
		}
		detail := "failed"
		if ok {
			detail = "passed"
		}
		assertions = append(assertions, AssertionResult{Name: check.Name, Kind: string(check.Kind), Category: checkCategory(check), Passed: ok, Detail: detail})
	}
	return float64(passed) / float64(len(item.Checks)), assertions
}

func validFailureClass(category FailureClass) bool {
	switch category {
	case FailureFactual, FailureFormat, FailureInstruction, FailureSafety:
		return true
	default:
		return false
	}
}

func checkCategory(check Check) FailureClass {
	if validFailureClass(check.Category) {
		return check.Category
	}
	switch check.Kind {
	case CheckJSONKeys, CheckMaxWords, CheckRegexp:
		return FailureFormat
	case CheckQuestion:
		return FailureInstruction
	default:
		return FailureFactual
	}
}

func failedClasses(assertions []AssertionResult) []FailureClass {
	seen := make(map[FailureClass]struct{})
	for _, assertion := range assertions {
		if !assertion.Passed {
			seen[assertion.Category] = struct{}{}
		}
	}
	order := []FailureClass{FailureSafety, FailureFactual, FailureFormat, FailureInstruction}
	classes := make([]FailureClass, 0, len(seen))
	for _, category := range order {
		if _, ok := seen[category]; ok {
			classes = append(classes, category)
		}
	}
	return classes
}

func checkAnswer(check Check, output string) bool {
	trimmed := strings.TrimSpace(output)
	lower := strings.ToLower(trimmed)
	switch check.Kind {
	case CheckContainsAll:
		for _, value := range check.Values {
			if !strings.Contains(lower, strings.ToLower(value)) {
				return false
			}
		}
		return true
	case CheckContainsAny:
		for _, value := range check.Values {
			if strings.Contains(lower, strings.ToLower(value)) {
				return true
			}
		}
		return false
	case CheckExcludesAll:
		for _, value := range check.Values {
			if strings.Contains(lower, strings.ToLower(value)) {
				return false
			}
		}
		return true
	case CheckExact:
		for _, value := range check.Values {
			if strings.EqualFold(trimmed, strings.TrimSpace(value)) {
				return true
			}
		}
		return false
	case CheckRegexp:
		re, err := regexp.Compile(check.Pattern)
		return err == nil && re.MatchString(trimmed)
	case CheckJSONKeys:
		var value map[string]any
		if json.Unmarshal([]byte(trimmed), &value) != nil {
			return false
		}
		for _, key := range check.Values {
			item, ok := value[key]
			if !ok || item == nil || strings.TrimSpace(fmt.Sprint(item)) == "" {
				return false
			}
		}
		return true
	case CheckMaxWords:
		limit, err := strconv.Atoi(check.Values[0])
		return err == nil && len(strings.Fields(trimmed)) <= limit
	case CheckQuestion:
		return strings.Contains(trimmed, "?")
	default:
		return false
	}
}

func routeScore(item Case, plan runtimekit.PlanView) float64 {
	if item.ExpectedExpert == "" {
		return 1
	}
	if plan.Selection.Primary == item.ExpectedExpert {
		return 1
	}
	return 0
}

func outputMetadata(output string, maxRunes int) (string, string) {
	digest := sha256.Sum256([]byte(output))
	redacted := redact(output)
	redacted = strings.Join(strings.Fields(redacted), " ")
	if maxRunes <= 0 {
		maxRunes = 240
	}
	if utf8.RuneCountInString(redacted) > maxRunes {
		runes := []rune(redacted)
		redacted = string(runes[:maxRunes]) + "…"
	}
	return hex.EncodeToString(digest[:]), redacted
}

func redact(value string) string {
	redacted := value
	for _, pattern := range sensitivePatterns {
		redacted = pattern.ReplaceAllString(redacted, "[REDACTED]")
	}
	return redacted
}
