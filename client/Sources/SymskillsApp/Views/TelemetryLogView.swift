import SwiftUI

struct TelemetryLogView: View {
    let runner: CLICommandRunner
    
    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("RAW SUBPROCESS CONSOLE")
                        .font(.caption2.monospaced())
                        .tracking(2)
                        .foregroundColor(Theme.goldPrimary)
                    Text("Telemetry Logs")
                        .font(.title2.bold())
                }
                Spacer()
                
                Button("Clear Console") {
                    runner.clearLogs()
                }
                .buttonStyle(SymairaSecondaryButtonStyle())
            }
            
            // Console display box
            ScrollViewReader { proxy in
                ScrollView([.vertical, .horizontal]) {
                    LazyVStack(alignment: .leading, spacing: 4) {
                        if runner.logs.isEmpty {
                            Text("No actions executed yet. Telemetry is idle.")
                                .foregroundColor(Theme.textMuted)
                                .font(.system(.body, design: .monospaced))
                                .padding(.top, 20)
                        } else {
                            ForEach(Array(runner.logs.enumerated()), id: \.offset) { index, line in
                                Text(line)
                                    .font(.system(.caption2, design: .monospaced))
                                    .foregroundColor(colorFor(line))
                                    .textSelection(.enabled)
                                    .id(index)
                            }
                        }
                    }
                    .padding(14)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
                .background(Color.black.opacity(0.4))
                .cornerRadius(8)
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(Theme.borderGlass, lineWidth: 1)
                )
                .onChange(of: runner.logs.count) { _, newValue in
                    if newValue > 0 {
                        proxy.scrollTo(newValue - 1, anchor: .bottom)
                    }
                }
                .task {
                    // Scroll to the end on load
                    if !runner.logs.isEmpty {
                        proxy.scrollTo(runner.logs.count - 1, anchor: .bottom)
                    }
                }
            }
        }
    }
    
    private func colorFor(_ line: String) -> Color {
        if line.contains("ERROR") || line.contains("Failed") || line.contains("exited with code") {
            return .red
        }
        if line.contains("Running:") {
            return Theme.goldPrimary
        }
        if line.contains("Completed successfully") {
            return .green
        }
        if line.contains("[stderr]") {
            return .orange
        }
        return Theme.textPrimary
    }
}
