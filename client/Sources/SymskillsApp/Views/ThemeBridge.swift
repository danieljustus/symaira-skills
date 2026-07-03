import SwiftUI
// Re-exported so all views see SymairaTheme's tokens, components
// (BlueprintGrid, AmbientGlows, TelemetryCorners, glassmorphicPanel,
// Symaira button styles) and Color(hex:) without per-file imports.
// This app was the donor of those components; the shared package took
// print's border opacities (0.06/0.22 vs the former 0.05/0.18) as the
// unified brand values.
@_exported import SymairaTheme

typealias Theme = SymairaTheme
