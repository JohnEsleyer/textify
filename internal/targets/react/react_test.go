package react

import (
	"strings"
	"testing"

	"github.com/JohnEsleyer/textify/internal/targets"
)

const reactSample = `import React, { useState } from 'react';

export interface HeaderProps {
	title: string;
	onClose: () => void;
}

export const Header: React.FC<HeaderProps> = ({ title, onClose }) => {
	const [active, setActive] = useState(false);
	return (
		<div className="header">
			<h1>{title}</h1>
			<button onClick={onClose}>Close</button>
		</div>
	);
};

export function useWindowSize() {
	return { width: 1024, height: 768 };
}

export class LegacyHeader extends React.Component<HeaderProps> {
	render() {
		return <div className="legacy">{this.props.title}</div>;
	}
}
`

func TestReactTarget(t *testing.T) {
	target := &ReactTarget{}
	if !target.Match("src/components/Header.tsx") {
		t.Errorf("ReactTarget should match .tsx file")
	}
	if !target.Match("src/utils.js") {
		t.Errorf("ReactTarget should match .js file")
	}

	syms, err := target.Parse("Header.tsx", []byte(reactSample))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	var foundProps, foundComp, foundHook, foundClass bool
	for _, s := range syms {
		switch s.Name {
		case "HeaderProps":
			foundProps = s.Kind == targets.KindInterface
		case "Header":
			foundComp = s.Kind == targets.KindComponent
		case "useWindowSize":
			foundHook = s.Kind == targets.KindHook
		case "LegacyHeader":
			foundClass = s.Kind == targets.KindComponent
		}
	}

	if !foundProps {
		t.Errorf("expected to find HeaderProps interface")
	}
	if !foundComp {
		t.Errorf("expected to find Header as KindComponent")
	}
	if !foundHook {
		t.Errorf("expected to find useWindowSize as KindHook")
	}
	if !foundClass {
		t.Errorf("expected to find LegacyHeader as a React component class")
	}

	sigs := target.ExtractSignatures("Header.tsx", reactSample)
	if !strings.Contains(sigs, "export interface HeaderProps") {
		t.Errorf("signatures missing HeaderProps: %s", sigs)
	}
	if !strings.Contains(sigs, "export const Header") || !strings.Contains(sigs, "/* component */") {
		t.Errorf("signatures missing Header component stub: %s", sigs)
	}
	if !strings.Contains(sigs, "function useWindowSize") || !strings.Contains(sigs, "/* hook */") {
		t.Errorf("signatures missing useWindowSize hook stub: %s", sigs)
	}
	if !strings.Contains(sigs, "render()") {
		t.Errorf("signatures should preserve class method stubs: %s", sigs)
	}
	// JSX body must be omitted.
	if strings.Contains(sigs, "<div className=\"header\">") {
		t.Errorf("signatures should not contain JSX markup: %s", sigs)
	}
	if strings.Contains(sigs, "useState(false)") {
		t.Errorf("signatures should not contain component body: %s", sigs)
	}
}
