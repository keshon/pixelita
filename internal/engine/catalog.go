package engine

type Capability struct {
	ID                 string   `json:"id"`
	Intents            []string `json:"intents"`
	InputFormats       []string `json:"inputFormats"`
	OutputFormat       string   `json:"outputFormat"`
	Lossiness          string   `json:"lossiness"`
	Dimensions         string   `json:"dimensions"`
	Alpha              string   `json:"alpha"`
	Pixels             string   `json:"pixels"`
	Metadata           string   `json:"metadata"`
	Verification       string   `json:"verification"`
	DestructiveEffects []string `json:"destructiveEffects,omitempty"`
	Automatic          bool     `json:"automatic"`
}

var catalog = []Capability{
	{ID: "jpeg-huffman", Intents: []string{"optimize"}, InputFormats: []string{"jpeg"}, OutputFormat: "same",
		Lossiness: "pixel-identical", Dimensions: "preserved", Alpha: "not-applicable", Pixels: "preserved",
		Metadata: "preserved", Verification: "decode", DestructiveEffects: []string{"overwrite-destination", "replace-source"}, Automatic: true},
	{ID: "png-palette", Intents: []string{"optimize"}, InputFormats: []string{"png"}, OutputFormat: "same",
		Lossiness: "lossy", Dimensions: "preserved", Alpha: "preserved", Pixels: "fidelity-threshold",
		Metadata: "not-preserved", Verification: "decode+psnr", DestructiveEffects: []string{"overwrite-destination", "replace-source"}, Automatic: true},
	{ID: "webp", Intents: []string{"optimize", "convert"}, InputFormats: []string{"png", "jpeg"}, OutputFormat: "webp",
		Lossiness: "configurable", Dimensions: "preserved", Alpha: "preserved", Pixels: "fidelity-threshold",
		Metadata: "not-preserved", Verification: "decode+psnr+ssim", DestructiveEffects: []string{"overwrite-destination", "delete-source"}, Automatic: false},
	{ID: "resize", Intents: []string{"variant"}, InputFormats: []string{"png", "jpeg"}, OutputFormat: "same",
		Lossiness: "resampled", Dimensions: "changed", Alpha: "preserved-unless-jpeg", Pixels: "resampled",
		Metadata: "not-preserved", Verification: "decode", DestructiveEffects: []string{"overwrite-destination", "replace-source"}, Automatic: false},
	{ID: "compare", Intents: []string{"compare", "verify"}, InputFormats: []string{"png", "jpeg", "webp"}, OutputFormat: "none",
		Lossiness: "none", Dimensions: "resampled-for-comparison", Alpha: "measured", Pixels: "measured",
		Metadata: "not-modified", Verification: "psnr+ssim", Automatic: false},
	{ID: "view", Intents: []string{"view", "inspect"}, InputFormats: []string{"png", "jpeg", "webp"}, OutputFormat: "png-preview",
		Lossiness: "presentation-only", Dimensions: "presentation-scaled", Alpha: "checkerboard-by-default", Pixels: "source-not-modified",
		Metadata: "not-modified", Verification: "decode", Automatic: false},
}

func Capabilities() []Capability {
	out := make([]Capability, len(catalog))
	for i, c := range catalog {
		c.Intents = append([]string(nil), c.Intents...)
		c.InputFormats = append([]string(nil), c.InputFormats...)
		c.DestructiveEffects = append([]string(nil), c.DestructiveEffects...)
		out[i] = c
	}
	return out
}

func capability(id string) (Capability, bool) {
	for _, c := range catalog {
		if c.ID == id {
			return c, true
		}
	}
	return Capability{}, false
}

func automaticCapability(format string) (Capability, bool) {
	for _, c := range catalog {
		if c.Automatic && supportsFormat(c, format) {
			return c, true
		}
	}
	return Capability{}, false
}

func supportsFormat(c Capability, format string) bool {
	for _, supported := range c.InputFormats {
		if supported == format {
			return true
		}
	}
	return false
}
