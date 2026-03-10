package ui

import "github.com/tri2820/cheese/client-toolkit/shell"

type LayerPosition = shell.LayerPosition

const (
	LayerBackground LayerPosition = shell.LayerPositionBackground
	LayerBottom     LayerPosition = shell.LayerPositionBottom
	LayerTop        LayerPosition = shell.LayerPositionTop
	LayerOverlay    LayerPosition = shell.LayerPositionOverlay
)

type LayerAnchor = shell.LayerAnchor

const (
	AnchorTop    LayerAnchor = shell.AnchorTop
	AnchorBottom LayerAnchor = shell.AnchorBottom
	AnchorLeft   LayerAnchor = shell.AnchorLeft
	AnchorRight  LayerAnchor = shell.AnchorRight
)
