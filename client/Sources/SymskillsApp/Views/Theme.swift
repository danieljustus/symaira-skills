import SwiftUI

public enum Theme {
    public static let bgDark = Color(hex: "0D0C0A")
    public static let bgDarker = Color(hex: "070605")
    public static let bgCard = Color(hex: "12110E").opacity(0.65)
    public static let bgCardHover = Color(hex: "1A1814").opacity(0.8)
    
    public static let goldPrimary = Color(hex: "E5C397")
    public static let goldSecondary = Color(hex: "F8E6CD")
    public static let goldShadow = Color(hex: "C29965")
    
    public static let icePrimary = Color(hex: "EEDCC4")
    public static let iceSecondary = Color(hex: "D4B285")
    
    public static let textPrimary = Color(hex: "F5F4F0")
    public static let textSecondary = Color(hex: "B5AEA5")
    public static let textMuted = Color(hex: "6E6860")
    
    public static let borderGlass = Color.white.opacity(0.05)
    public static let borderGlassHover = Color(hex: "E5C397").opacity(0.18)
    
    public static let transitionSmooth = Animation.timingCurve(0.16, 1, 0.3, 1, duration: 0.4)
    public static let transitionFast = Animation.easeOut(duration: 0.2)
}

extension Color {
    public init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let a, r, g, b: UInt64
        switch hex.count {
        case 3: // RGB (12-bit)
            (a, r, g, b) = (255, (int >> 8) * 17, (int >> 4 & 0xF) * 17, (int & 0xF) * 17)
        case 6: // RGB (24-bit)
            (a, r, g, b) = (255, int >> 16, int >> 8 & 0xFF, int & 0xFF)
        case 8: // ARGB (32-bit)
            (a, r, g, b) = (int >> 24, int >> 16 & 0xFF, int >> 8 & 0xFF, int & 0xFF)
        default:
            (a, r, g, b) = (255, 0, 0, 0)
        }
        self.init(
            .sRGB,
            red: Double(r) / 255,
            green: Double(g) / 255,
            blue:  Double(b) / 255,
            opacity: Double(a) / 255
        )
    }
}

// MARK: - Blueprint Background Grid
public struct BlueprintGrid: View {
    public init() {}
    public var body: some View {
        GeometryReader { geo in
            Path { path in
                let spacing: CGFloat = 30
                // Draw horizontal lines
                var y: CGFloat = 0
                while y < geo.size.height {
                    path.move(to: CGPoint(x: 0, y: y))
                    path.addLine(to: CGPoint(x: geo.size.width, y: y))
                    y += spacing
                }
                // Draw vertical lines
                var x: CGFloat = 0
                while x < geo.size.width {
                    path.move(to: CGPoint(x: x, y: 0))
                    path.addLine(to: CGPoint(x: x, y: geo.size.height))
                    x += spacing
                }
            }
            .stroke(Color.white.opacity(0.015), lineWidth: 0.5)
        }
    }
}

// MARK: - Ambient Glows Background
public struct AmbientGlows: View {
    public init() {}
    public var body: some View {
        ZStack {
            // Gold glow top left
            Circle()
                .fill(Theme.goldPrimary.opacity(0.04))
                .frame(width: 450, height: 450)
                .blur(radius: 90)
                .offset(x: -200, y: -200)
            
            // Warm sand glow bottom right
            Circle()
                .fill(Theme.goldSecondary.opacity(0.03))
                .frame(width: 550, height: 550)
                .blur(radius: 110)
                .offset(x: 250, y: 250)
        }
    }
}

// MARK: - Telemetry Corners
public struct TelemetryCorners: View {
    var color: Color
    var length: CGFloat
    var lineWidth: CGFloat
    
    public init(color: Color = Theme.goldPrimary.opacity(0.35), length: CGFloat = 8, lineWidth: CGFloat = 1) {
        self.color = color
        self.length = length
        self.lineWidth = lineWidth
    }
    
    public var body: some View {
        GeometryReader { geo in
            Path { path in
                // Top-Left
                path.move(to: CGPoint(x: 0, y: length))
                path.addLine(to: CGPoint(x: 0, y: 0))
                path.addLine(to: CGPoint(x: length, y: 0))
                
                // Top-Right
                path.move(to: CGPoint(x: geo.size.width - length, y: 0))
                path.addLine(to: CGPoint(x: geo.size.width, y: 0))
                path.addLine(to: CGPoint(x: geo.size.width, y: length))
                
                // Bottom-Left
                path.move(to: CGPoint(x: 0, y: geo.size.height - length))
                path.addLine(to: CGPoint(x: 0, y: geo.size.height))
                path.addLine(to: CGPoint(x: length, y: geo.size.height))
                
                // Bottom-Right
                path.move(to: CGPoint(x: geo.size.width - length, y: geo.size.height))
                path.addLine(to: CGPoint(x: geo.size.width, y: geo.size.height))
                path.addLine(to: CGPoint(x: geo.size.width, y: geo.size.height - length))
            }
            .stroke(color, lineWidth: lineWidth)
        }
    }
}

// MARK: - Glassmorphic Panel Modifier
public struct GlassmorphicPanelModifier: ViewModifier {
    var cornerRadius: CGFloat
    var addCorners: Bool
    
    public init(cornerRadius: CGFloat = 12, addCorners: Bool = true) {
        self.cornerRadius = cornerRadius
        self.addCorners = addCorners
    }
    
    public func body(content: Content) -> some View {
        content
            .background(Theme.bgCard)
            .overlay(
                RoundedRectangle(cornerRadius: cornerRadius)
                    .stroke(Theme.borderGlass, lineWidth: 1)
            )
            .overlay(
                Group {
                    if addCorners {
                        TelemetryCorners(color: Theme.goldPrimary.opacity(0.35), length: 8, lineWidth: 1)
                    }
                }
            )
            .cornerRadius(cornerRadius)
            .shadow(color: Color.black.opacity(0.35), radius: 12, x: 0, y: 6)
    }
}

extension View {
    public func glassmorphicPanel(cornerRadius: CGFloat = 12, addCorners: Bool = true) -> some View {
        self.modifier(GlassmorphicPanelModifier(cornerRadius: cornerRadius, addCorners: addCorners))
    }
}

// MARK: - Premium Button Styles
public struct SymairaPrimaryButtonStyle: ButtonStyle {
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline.weight(.semibold))
            .foregroundColor(.black)
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
            .background(
                LinearGradient(
                    colors: [Theme.goldPrimary, Theme.goldSecondary],
                    startPoint: .leading,
                    endPoint: .trailing
                )
            )
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(Color.white.opacity(0.15), lineWidth: 1)
            )
            .scaleEffect(configuration.isPressed ? 0.97 : 1.0)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
            .shadow(color: Theme.goldPrimary.opacity(0.25), radius: configuration.isPressed ? 4 : 8, x: 0, y: 2)
    }
}

public struct SymairaSecondaryButtonStyle: ButtonStyle {
    public init() {}
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline.weight(.medium))
            .foregroundColor(Theme.textPrimary)
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
            .background(Color.white.opacity(0.04))
            .clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(Theme.borderGlass, lineWidth: 1)
            )
            .scaleEffect(configuration.isPressed ? 0.97 : 1.0)
            .animation(.easeOut(duration: 0.15), value: configuration.isPressed)
    }
}
