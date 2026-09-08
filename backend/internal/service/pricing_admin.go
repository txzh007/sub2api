package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ModelPricingCatalogEntry is the administrator-facing effective pricing view.
// Prices use the catalog's per-token numeric unit. The UI labels international
// model families as USD and other providers as RMB without FX conversion.
type ModelPricingCatalogEntry struct {
	Model                               string         `json:"model"`
	LiteLLMProvider                     string         `json:"litellm_provider"`
	Mode                                string         `json:"mode"`
	InputCostPerToken                   float64        `json:"input_cost_per_token"`
	InputCostPerTokenPriority           float64        `json:"input_cost_per_token_priority"`
	OutputCostPerToken                  float64        `json:"output_cost_per_token"`
	OutputCostPerTokenPriority          float64        `json:"output_cost_per_token_priority"`
	CacheCreationInputTokenCost         float64        `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostPriority float64        `json:"cache_creation_input_token_cost_priority"`
	CacheCreationInputTokenCostAbove1hr float64        `json:"cache_creation_input_token_cost_above_1hr"`
	CacheReadInputTokenCost             float64        `json:"cache_read_input_token_cost"`
	CacheReadInputTokenCostPriority     float64        `json:"cache_read_input_token_cost_priority"`
	OutputCostPerImage                  float64        `json:"output_cost_per_image"`
	OutputCostPerImageToken             float64        `json:"output_cost_per_image_token"`
	InputCostPerImageToken              float64        `json:"input_cost_per_image_token"`
	LongContextInputTokenThreshold      int            `json:"long_context_input_token_threshold"`
	LongContextInputCostMultiplier      float64        `json:"long_context_input_cost_multiplier"`
	LongContextOutputCostMultiplier     float64        `json:"long_context_output_cost_multiplier"`
	SupportsPromptCaching               bool           `json:"supports_prompt_caching"`
	SupportsServiceTier                 bool           `json:"supports_service_tier"`
	TokenPricingAbsent                  bool           `json:"token_pricing_absent"`
	Overridden                          bool           `json:"overridden"`
	Wildcard                            bool           `json:"wildcard"`
	Override                            map[string]any `json:"override,omitempty"`
}

type ModelPricingCatalog struct {
	Items         []ModelPricingCatalogEntry `json:"items"`
	ModelCount    int                        `json:"model_count"`
	OverrideCount int                        `json:"override_count"`
	LastUpdated   time.Time                  `json:"last_updated"`
	OverrideFile  string                     `json:"override_file"`
}

// PricingOverrideValidationError marks a user-editable pricing payload error.
type PricingOverrideValidationError struct{ message string }

func (e *PricingOverrideValidationError) Error() string { return e.message }

func IsPricingOverrideValidationError(err error) bool {
	var target *PricingOverrideValidationError
	return errors.As(err, &target)
}

var pricingCostFields = map[string]struct{}{
	"input_cost_per_token":                      {},
	"input_cost_per_token_priority":             {},
	"output_cost_per_token":                     {},
	"output_cost_per_token_priority":            {},
	"cache_creation_input_token_cost":           {},
	"cache_creation_input_token_cost_priority":  {},
	"cache_creation_input_token_cost_above_1hr": {},
	"cache_read_input_token_cost":               {},
	"cache_read_input_token_cost_priority":      {},
	"output_cost_per_image":                     {},
	"output_cost_per_image_token":               {},
	"input_cost_per_image_token":                {},
}

func (s *PricingService) pricingOverridePath() (string, error) {
	if s == nil || s.cfg == nil {
		return "", fmt.Errorf("pricing service is not configured")
	}
	path := strings.TrimSpace(s.cfg.Pricing.OverrideFile)
	if path == "" {
		return "", fmt.Errorf("pricing override file is not configured")
	}
	return path, nil
}

func validatePricingModelPattern(model string) error {
	if model == "" {
		return &PricingOverrideValidationError{message: "model is required"}
	}
	if len(model) > 255 {
		return &PricingOverrideValidationError{message: "model must not exceed 255 characters"}
	}
	for _, r := range model {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return &PricingOverrideValidationError{message: "model must not contain whitespace or control characters"}
		}
	}
	if model == "*" {
		return &PricingOverrideValidationError{message: "bare wildcard '*' is not allowed"}
	}
	if strings.Count(model, "*") > 1 || (strings.Contains(model, "*") && !strings.HasSuffix(model, "*")) {
		return &PricingOverrideValidationError{message: "wildcard is only allowed once at the end of the model name"}
	}
	return nil
}

func validatePricingOverridePatch(patch map[string]any) error {
	if len(patch) == 0 {
		return &PricingOverrideValidationError{message: "pricing override is required"}
	}
	hasCost := false
	for field, value := range patch {
		if value == nil {
			continue
		}
		if _, ok := pricingCostFields[field]; ok || strings.Contains(field, "_cost_per_") {
			number, ok := value.(float64)
			if !ok {
				return &PricingOverrideValidationError{message: fmt.Sprintf("%s must be a number or null", field)}
			}
			if number < 0 {
				return &PricingOverrideValidationError{message: fmt.Sprintf("%s must not be negative", field)}
			}
			hasCost = true
		}
	}
	if !hasCost {
		return &PricingOverrideValidationError{message: "at least one price field is required"}
	}
	return nil
}

func readPricingOverrideFile(path string) (map[string]json.RawMessage, []byte, bool, error) {
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]json.RawMessage), nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("read pricing override file: %w", err)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, body, true, fmt.Errorf("parse pricing override file: %w", err)
	}
	if entries == nil {
		return nil, body, true, fmt.Errorf("parse pricing override file: root must be a JSON object")
	}
	return entries, body, true, nil
}

func writePricingOverrideFile(path string, entries map[string]json.RawMessage) error {
	body, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("encode pricing override file: %w", err)
	}
	body = append(body, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create pricing override directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".pricing-overrides-*")
	if err != nil {
		return fmt.Errorf("create temporary pricing override file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		return fmt.Errorf("set pricing override permissions: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("write pricing override file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync pricing override file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close pricing override file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace pricing override file: %w", err)
	}
	return nil
}

func restorePricingOverrideFile(path string, oldBody []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(oldBody, &entries); err != nil {
		return err
	}
	return writePricingOverrideFile(path, entries)
}

func (s *PricingService) mutatePricingOverrides(mutate func(map[string]json.RawMessage) error) error {
	path, err := s.pricingOverridePath()
	if err != nil {
		return err
	}
	s.overrideWriteMu.Lock()
	defer s.overrideWriteMu.Unlock()

	entries, oldBody, existed, err := readPricingOverrideFile(path)
	if err != nil {
		return err
	}
	if err := mutate(entries); err != nil {
		return err
	}
	if err := writePricingOverrideFile(path, entries); err != nil {
		return err
	}
	if err := s.reloadCustomPricingLayers(); err != nil {
		rollbackErr := restorePricingOverrideFile(path, oldBody, existed)
		if rollbackErr == nil {
			_ = s.reloadCustomPricingLayers()
		}
		if rollbackErr != nil {
			return fmt.Errorf("reload pricing after save: %w; rollback failed: %v", err, rollbackErr)
		}
		return fmt.Errorf("reload pricing after save: %w", err)
	}
	return nil
}

// UpsertPricingOverride creates or field-merges a local override and applies it immediately.
func (s *PricingService) UpsertPricingOverride(model string, patch map[string]any) error {
	model = strings.TrimSpace(model)
	if err := validatePricingModelPattern(model); err != nil {
		return err
	}
	if err := validatePricingOverridePatch(patch); err != nil {
		return err
	}
	patchRaw, err := json.Marshal(patch)
	if err != nil {
		return &PricingOverrideValidationError{message: "pricing override is not valid JSON"}
	}
	return s.mutatePricingOverrides(func(entries map[string]json.RawMessage) error {
		merged, ok := mergePricingOverrideEntry(entries[model], patchRaw)
		if !ok {
			return &PricingOverrideValidationError{message: "pricing override must be a JSON object"}
		}
		entries[model] = merged
		return nil
	})
}

// DeletePricingOverride removes a local override and restores the underlying catalog price.
func (s *PricingService) DeletePricingOverride(model string) error {
	model = strings.TrimSpace(model)
	if err := validatePricingModelPattern(model); err != nil {
		return err
	}
	return s.mutatePricingOverrides(func(entries map[string]json.RawMessage) error {
		delete(entries, model)
		return nil
	})
}

// ListAdminModelPricing returns a stable snapshot of effective pricing and local override metadata.
func (s *PricingService) ListAdminModelPricing() (*ModelPricingCatalog, error) {
	path, err := s.pricingOverridePath()
	if err != nil {
		return nil, err
	}
	s.overrideWriteMu.Lock()
	defer s.overrideWriteMu.Unlock()
	overrides, _, _, err := readPricingOverrideFile(path)
	if err != nil {
		return nil, err
	}
	overrideMaps := make(map[string]map[string]any, len(overrides))
	for model, raw := range overrides {
		var fields map[string]any
		if json.Unmarshal(raw, &fields) == nil {
			overrideMaps[model] = fields
		}
	}

	s.mu.RLock()
	items := make([]ModelPricingCatalogEntry, 0, len(s.pricingData))
	for model, p := range s.pricingData {
		if p == nil {
			continue
		}
		_, overridden := overrides[model]
		items = append(items, ModelPricingCatalogEntry{
			Model: model, LiteLLMProvider: p.LiteLLMProvider, Mode: p.Mode,
			InputCostPerToken: p.InputCostPerToken, InputCostPerTokenPriority: p.InputCostPerTokenPriority,
			OutputCostPerToken: p.OutputCostPerToken, OutputCostPerTokenPriority: p.OutputCostPerTokenPriority,
			CacheCreationInputTokenCost:         p.CacheCreationInputTokenCost,
			CacheCreationInputTokenCostPriority: p.CacheCreationInputTokenCostPriority,
			CacheCreationInputTokenCostAbove1hr: p.CacheCreationInputTokenCostAbove1hr,
			CacheReadInputTokenCost:             p.CacheReadInputTokenCost,
			CacheReadInputTokenCostPriority:     p.CacheReadInputTokenCostPriority,
			OutputCostPerImage:                  p.OutputCostPerImage, OutputCostPerImageToken: p.OutputCostPerImageToken,
			InputCostPerImageToken:          p.InputCostPerImageToken,
			LongContextInputTokenThreshold:  p.LongContextInputTokenThreshold,
			LongContextInputCostMultiplier:  p.LongContextInputCostMultiplier,
			LongContextOutputCostMultiplier: p.LongContextOutputCostMultiplier,
			SupportsPromptCaching:           p.SupportsPromptCaching, SupportsServiceTier: p.SupportsServiceTier,
			TokenPricingAbsent: p.TokenPricingAbsent, Overridden: overridden,
			Wildcard: strings.HasSuffix(model, "*"), Override: overrideMaps[model],
		})
	}
	lastUpdated := s.lastUpdated
	s.mu.RUnlock()
	sort.Slice(items, func(i, j int) bool { return strings.ToLower(items[i].Model) < strings.ToLower(items[j].Model) })
	return &ModelPricingCatalog{
		Items: items, ModelCount: len(items), OverrideCount: len(overrides),
		LastUpdated: lastUpdated, OverrideFile: path,
	}, nil
}
