package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/npillmayer/fontfind"
	"github.com/npillmayer/fontfind/locate/fallbackfont"
	"golang.org/x/image/font"
	"tytui/fontbackend"
)

func offlineFontDiscovery() *fontbackend.Discovery {
	d, err := fontbackend.NewDiscovery([]fontbackend.OfflineProvider{{Name: "packaged-catalog", Find: func(ctx context.Context, desc fontfind.Descriptor) (fontbackend.Match, error) {
		if err := ctx.Err(); err != nil {
			return fontbackend.Match{}, err
		}
		sf, err := fallbackfont.FindFallbackFont("Gentium", font.StyleNormal, font.WeightNormal)
		if err != nil {
			return fontbackend.Match{}, err
		}
		if sf.Name != "GentiumPlus-R.ttf" {
			return fontbackend.Match{}, fmt.Errorf("unexpected packaged catalog face %q", sf.Name)
		}
		exact := (strings.EqualFold(desc.Pattern, "Gentium Plus") || strings.EqualFold(desc.Pattern, "Gentium")) && desc.Style == font.StyleNormal && desc.Weight == font.WeightNormal
		reason := "packaged Gentium Plus Regular"
		if !exact {
			reason = "catalog has only Gentium Plus Regular; requested face unavailable"
		}
		return fontbackend.Match{Font: sf, Exact: exact, Reason: reason}, nil
	}}}, map[string]string{"serif": "Gentium Plus"})
	if err != nil {
		panic(err)
	}
	return d
}

// Requests use explicit semicolon fields; each comma-separated entry is a family
// name, not CSS syntax (quoted names containing commas are outside this UI subset).
func parseFontRequest(value string, substitute bool) (fontbackend.FontRequest, error) {
	parts := strings.Split(value, ";")
	if len(parts) != 3 {
		return fontbackend.FontRequest{}, fmt.Errorf("use families ; weight ; normal/italic/oblique")
	}
	weight, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || weight < 100 || weight > 900 || weight%100 != 0 {
		return fontbackend.FontRequest{}, fmt.Errorf("request weight must be 100..900 in steps of 100")
	}
	slant := strings.TrimSpace(parts[2])
	if slant != "normal" && slant != "italic" && slant != "oblique" {
		return fontbackend.FontRequest{}, fmt.Errorf("invalid request slant")
	}
	families := strings.Split(parts[0], ",")
	for i := range families {
		families[i] = strings.TrimSpace(families[i])
		if families[i] == "" {
			return fontbackend.FontRequest{}, fmt.Errorf("empty family")
		}
	}
	return fontbackend.FontRequest{Families: families, Weight: weight, Slant: slant, AllowSubstitution: substitute}, nil
}
func (f *fontEditor) descriptorSettings() (fontbackend.DescriptorSettings, error) {
	// Reuse mapping predicates and ordering, interpreting target columns as requests.
	s, err := f.settings()
	if err != nil {
		return fontbackend.DescriptorSettings{}, err
	}
	input := fontbackend.DescriptorSettings{Size: s.Size}
	input.Default, err = parseFontRequest(s.DefaultPath, f.substitute)
	if err != nil {
		return input, err
	}
	for _, rule := range s.Rules {
		request, err := parseFontRequest(rule.Path, f.substitute)
		if err != nil {
			return input, err
		}
		input.Rules = append(input.Rules, fontbackend.DescriptorRule{Family: rule.Family, Weight: rule.Weight, Slant: rule.Slant, Request: request})
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
func fontReport(infos []fontbackend.Info, diagnostics []fontbackend.FontDiagnostic) string {
	var report strings.Builder
	for _, d := range diagnostics {
		label := d.Stage
		if d.Substituted {
			label += " SUBSTITUTION"
		}
		slant := "normal"
		if d.Request.Style == font.StyleItalic {
			slant = "italic"
		} else if d.Request.Style == font.StyleOblique {
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
