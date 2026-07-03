import SwiftUI

struct DiffView: View {
    let target: String
    let changes: [Change]
    @Environment(\.dismiss) private var dismiss
    
    var body: some View {
        NavigationStack {
            ZStack {
                Theme.bgDark.ignoresSafeArea()
                BlueprintGrid()
                
                VStack(alignment: .leading, spacing: 20) {
                    if changes.isEmpty {
                        VStack(spacing: 12) {
                            Image(systemName: "checkmark.circle.fill")
                                .font(.system(size: 48))
                                .foregroundColor(.green)
                            Text("No Differences")
                                .font(.title3.bold())
                            Text("Rendered skill matches the installed target exactly.")
                                .foregroundColor(Theme.textSecondary)
                                .multilineTextAlignment(.center)
                        }
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                    } else {
                        Text("COMPARING RENDERED WITH INSTALLED")
                            .font(.caption2.monospaced())
                            .foregroundColor(Theme.goldPrimary)
                        
                        ScrollView {
                            VStack(spacing: 8) {
                                ForEach(changes) { change in
                                    HStack {
                                        Image(systemName: iconFor(change.status))
                                            .foregroundColor(colorFor(change.status))
                                            .font(.title3)
                                        
                                        Text(change.path)
                                            .font(.body.monospaced())
                                            .lineLimit(1)
                                            .truncationMode(.middle)
                                        
                                        Spacer()
                                        
                                        Text(change.status.uppercased())
                                            .font(.caption2.bold().monospaced())
                                            .padding(.horizontal, 8)
                                            .padding(.vertical, 4)
                                            .background(colorFor(change.status).opacity(0.12))
                                            .foregroundColor(colorFor(change.status))
                                            .clipShape(Capsule())
                                    }
                                    .padding(14)
                                    .glassmorphicPanel(addCorners: false)
                                }
                            }
                        }
                    }
                }
                .padding(24)
            }
            .navigationTitle("Diff — \(target)")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
        }
    }
    
    private func iconFor(_ status: String) -> String {
        switch status.lowercased() {
        case "added", "new": return "plus.circle.fill"
        case "deleted", "removed": return "minus.circle.fill"
        default: return "pencil.circle.fill"
        }
    }
    
    private func colorFor(_ status: String) -> Color {
        switch status.lowercased() {
        case "added", "new": return .green
        case "deleted", "removed": return .red
        default: return .orange
        }
    }
}
