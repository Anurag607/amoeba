package ollama

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/anurgosw/agentic-moe/moe"
)

type tagsResponse struct {
	Models []model `json:"models"`
}

type model struct {
	Name    string `json:"name"`
	Model   string `json:"model"`
	Size    int64  `json:"size"`
	Details struct {
		ParameterSize     string `json:"parameter_size"`
		QuantizationLevel string `json:"quantization_level"`
	} `json:"details"`
}

var parameterPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([bm])`)

func tierFor(m model) moe.ModelTier {
	source := m.Details.ParameterSize
	if source == "" {
		source = m.Name
	}
	match := parameterPattern.FindStringSubmatch(source)
	if len(match) != 3 {
		return moe.ModelTierBalanced
	}
	value, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return moe.ModelTierBalanced
	}
	if strings.EqualFold(match[2], "m") {
		value /= 1000
	}
	switch {
	case value <= 4:
		return moe.ModelTierFast
	case value <= 14:
		return moe.ModelTierBalanced
	default:
		return moe.ModelTierStrong
	}
}
