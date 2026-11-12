package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/fontfind/locate/fallbackfont"
	"github.com/npillmayer/typography"
	findadapter "github.com/npillmayer/typography/adapter/fontfind"
	otadapter "github.com/npillmayer/typography/adapter/opentype"
	fontdiscovery "github.com/npillmayer/typography/discovery"
	"github.com/npillmayer/typography/fontresource"
	"golang.org/x/image/font"
)

// Compose the controlled catalog and OpenType loader explicitly in the application.
func offlineFontDiscovery() *fontdiscovery.Discovery {
	search, err := findadapter.New([]findadapter.OfflineProvider{{Name: "packaged-catalog", Find: func(ctx context.Context, desc fontfind.Descriptor) (findadapter.Match, error) {
		if err := ctx.Err(); err != nil {
			return findadapter.Match{}, err
		}
		sf, err := fallbackfont.FindFallbackFont("Gentium", font.StyleNormal, font.WeightNormal)
		if err != nil {
			return findadapter.Match{}, err
		}
		if sf.Name != "GentiumPlus-R.ttf" {
			return findadapter.Match{}, fmt.Errorf("unexpected packaged catalog face %q", sf.Name)
		}
		exact := (strings.EqualFold(desc.Pattern, "Gentium Plus") || strings.EqualFold(desc.Pattern, "Gentium")) && desc.Style == font.StyleNormal && desc.Weight == font.WeightNormal
		reason := "packaged Gentium Plus Regular"
		if !exact {
			reason = "catalog has only Gentium Plus Regular; requested face unavailable"
		}
		return findadapter.Match{Font: sf, Exact: exact, Reason: reason}, nil
	}}})
	if err != nil {
		panic(err)
	}
	d, err := fontdiscovery.New(search, otadapter.Services{}, map[string]string{"serif": "Gentium Plus"})
	if err != nil {
		panic(err)
	}
	return d
}

// Requests use explicit semicolon fields; each comma-separated entry is a family
// name, not CSS syntax (quoted names containing commas are outside this UI subset).
func parseFontRequest(value string, substitute bool) (fontdiscovery.FontRequest, error) {
	parts := strings.Split(value, ";")
	if len(parts) != 3 {
		return fontdiscovery.FontRequest{}, fmt.Errorf("use families ; weight ; normal/italic/oblique")
	}
	weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || weight < 100 || weight > 900 || weight%100 != 0 {
		return fontdiscovery.FontRequest{}, fmt.Errorf("request weight must be 100..900 in steps of 100")
	}
	slant := strings.TrimSpace(parts[2])
	if slant != "normal" && slant != "italic" && slant != "oblique" {
		return fontdiscovery.FontRequest{}, fmt.Errorf("invalid request slant")
	}
	families := strings.Split(parts[0], ",")
	for i := range families {
		families[i] = strings.TrimSpace(families[i])
		if families[i] == "" {
			return fontdiscovery.FontRequest{}, fmt.Errorf("empty family")
		}
	}
	return fontdiscovery.FontRequest{Families: families, Weight: weight, Slant: slant, AllowSubstitution: substitute}, nil
}
func (f *fontEditor) descriptorSettings() (fontdiscovery.DescriptorSettings, error) {
	// Reuse mapping predicates and ordering, interpreting target columns as requests.
	s, err := f.settings()
	if err != nil {
		return fontdiscovery.DescriptorSettings{}, err
	}
	input := fontdiscovery.DescriptorSettings{Size: s.Size}
	input.Default, err = parseFontRequest(s.DefaultPath, f.substitute)
	if err != nil {
		return input, err
	}
	for _, rule := range s.Rules {
		request, err := parseFontRequest(rule.Path, f.substitute)
		if err != nil {
			return input, err
		}
		input.Rules = append(input.Rules, fontdiscovery.DescriptorRule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, Request: request})
	}
	for _, value := range s.Fallbacks {
		request, err := parseFontRequest(value, f.substitute)
		if err != nil {
			return input, err
		}
		input.Fallbacks = append(input.Fallbacks, request)
	}
	return input, nil
}
func fontReport(infos []typography.Info, diagnostics []fontdiscovery.FontDiagnostic) string {
	var report strings.Builder
	for _, d := range diagnostics {
		label := d.Stage
		if d.Substituted {
			label += " SUBSTITUTION"
		}
		slant := "normal"
		if d.Request.Style == fontresource.Italic {
			slant = "italic"
		} else if d.Request.Style == fontresource.Oblique {
			slant = "oblique"
		}
		fmt.Fprintf(&report, "%s · requested %q %s %d · %s: %s\n", label, d.Request.Pattern, slant, int(d.Request.Weight)*100+400, d.Provider, d.Message)
	}
	for _, info := range infos {
		fmt.Fprintf(&report, "Font %q · %s %s\nSource %s · %s\nSHA-256 %s\n", info.Path, info.Family, info.Subfamily, info.Origin.Provider, info.Origin.Name, info.SHA256)
		for _, warning := range info.Warnings {
			fmt.Fprintf(&report, "Warning: %s\n", warning)
		}
	}
	return report.String()
}
