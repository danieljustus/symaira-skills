import SwiftUI

struct ContentView: View {
    @State private var runner = CLICommandRunner()
    @State private var selectedTab: Tab = .dashboard
    
    enum Tab {
        case dashboard
        case library
        case logs
    }
    
    var body: some View {
        NavigationSplitView {
            List(selection: $selectedTab) {
                NavigationLink(value: Tab.dashboard) {
                    Label("Dashboard", systemImage: "gauge.with.needle")
                }
                .tag(Tab.dashboard)
                
                NavigationLink(value: Tab.library) {
                    Label("Skill Library", systemImage: "square.grid.3x3.fill")
                }
                .tag(Tab.library)
                
                NavigationLink(value: Tab.logs) {
                    Label("Telemetry Logs", systemImage: "terminal.fill")
                }
                .tag(Tab.logs)
            }
            .listStyle(.sidebar)
            .navigationTitle("symskills")
            .navigationSplitViewColumnWidth(min: 200, ideal: 220, max: 280)
            
            // Bottom status bar in sidebar
            .safeAreaInset(edge: .bottom) {
                HStack {
                    Circle()
                        .fill(runner.isRunning ? Color.green : Color.secondary)
                        .frame(width: 8, height: 8)
                    Text(runner.isRunning ? "RUNNING ACTION" : "IDLE")
                        .font(.caption2.monospaced())
                        .foregroundColor(Theme.textSecondary)
                    Spacer()
                }
                .padding(12)
                .background(Color.black.opacity(0.2))
                .border(Theme.borderGlass, width: 1)
            }
        } detail: {
            ZStack {
                Theme.bgDark.ignoresSafeArea()
                BlueprintGrid()
                AmbientGlows()
                
                Group {
                    switch selectedTab {
                    case .dashboard:
                        DashboardView(runner: runner)
                    case .library:
                        LibraryView(runner: runner)
                    case .logs:
                        TelemetryLogView(runner: runner)
                    }
                }
                .padding(24)
            }
            .navigationTitle(titleForTab(selectedTab))
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    if runner.isRunning {
                        ProgressView()
                            .controlSize(.small)
                    }
                }
            }
        }
        .frame(minWidth: 900, minHeight: 650)
        .foregroundColor(Theme.textPrimary)
    }
    
    private func titleForTab(_ tab: Tab) -> String {
        switch tab {
        case .dashboard: return "System Dashboard"
        case .library: return "Skill Library"
        case .logs: return "Telemetry Logs"
        }
    }
}
