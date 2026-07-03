import SwiftUI

struct DashboardView: View {
    let runner: CLICommandRunner
    
    @State private var info: DoctorInfo?
    @State private var errorMessage: String?
    @State private var isInitializing = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            if isInitializing {
                VStack(spacing: 16) {
                    ProgressView("Initializing symskills environment…")
                    Text("Creating directories & defaults in ~/.config/symskills")
                        .foregroundColor(Theme.textSecondary)
                        .font(.subheadline)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let errorMessage = errorMessage, info == nil {
                // If it failed, check if we need to initialize
                VStack(spacing: 20) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 48))
                        .foregroundColor(Theme.goldPrimary)
                    
                    Text("Environment Not Initialized")
                        .font(.title2.bold())
                    
                    Text("The symskills configuration file or local share folders could not be found. Please initialize the environment.")
                        .foregroundColor(Theme.textSecondary)
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 32)
                    
                    Button("Initialize Environment") {
                        Task { await initializeEnv() }
                    }
                    .buttonStyle(SymairaPrimaryButtonStyle())
                    
                    Text(errorMessage)
                        .font(.caption2.monospaced())
                        .foregroundColor(.red)
                        .padding(.top, 10)
                }
                .padding(40)
                .glassmorphicPanel()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let info = info {
                // Render the dashboard info!
                ScrollView {
                    VStack(alignment: .leading, spacing: 24) {
                        // Title header
                        HStack {
                            VStack(alignment: .leading, spacing: 4) {
                                Text("SYSTEM OVERVIEW")
                                    .font(.caption2.monospaced())
                                    .tracking(2)
                                    .foregroundColor(Theme.goldPrimary)
                                Text("symskills environment")
                                    .font(.title.bold())
                            }
                            Spacer()
                            Button {
                                Task { await loadInfo() }
                            } label: {
                                Image(systemName: "arrow.clockwise")
                            }
                            .buttonStyle(SymairaSecondaryButtonStyle())
                        }
                        
                        // Active Configuration File Panel
                        VStack(alignment: .leading, spacing: 12) {
                            Text("CONFIGURATION FILE")
                                .font(.caption.bold().monospaced())
                                .foregroundColor(Theme.goldPrimary)
                            
                            HStack {
                                Image(systemName: "doc.text.fill")
                                    .foregroundColor(Theme.goldPrimary)
                                    .frame(width: 24)
                                Text(info.configPath)
                                    .font(.body.monospaced())
                                    .lineLimit(1)
                                    .truncationMode(.middle)
                                Spacer()
                                Button("Open Folder") {
                                    openFolder(at: (info.configPath as NSString).deletingLastPathComponent)
                                }
                                .buttonStyle(SymairaSecondaryButtonStyle())
                            }
                        }
                        .padding(20)
                        .glassmorphicPanel()
                        
                        // Core Directories (Grid layout)
                        Text("MANAGED DIRECTORIES")
                            .font(.caption.bold().monospaced())
                            .foregroundColor(Theme.goldPrimary)
                            .padding(.top, 8)
                        
                        LazyVGrid(columns: [GridItem(.flexible(), spacing: 16), GridItem(.flexible(), spacing: 16)], spacing: 16) {
                            DirectoryCard(title: "Skill Library", path: info.config.libraryDir, icon: "square.grid.3x3.fill")
                            DirectoryCard(title: "Render Cache", path: info.config.renderDir, icon: "doc.plaintext.fill")
                            DirectoryCard(title: "System Cache", path: info.config.cacheDir, icon: "folder.badge.gearshape")
                        }
                        
                        // Target Harness Installations
                        Text("TARGET INSTALLATION PATHS")
                            .font(.caption.bold().monospaced())
                            .foregroundColor(Theme.goldPrimary)
                            .padding(.top, 12)
                        
                        VStack(spacing: 0) {
                            ForEach(Array(info.targets.enumerated()), id: \.offset) { index, target in
                                TargetPathRow(target: target)
                                if index < info.targets.count - 1 {
                                    Divider()
                                        .background(Theme.borderGlass)
                                }
                            }
                        }
                        .glassmorphicPanel(addCorners: false)
                    }
                }
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task {
            await loadInfo()
        }
    }
    
    private func loadInfo() async {
        do {
            errorMessage = nil
            info = try await runner.doctor()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
    
    private func initializeEnv() async {
        isInitializing = true
        defer { isInitializing = false }
        do {
            let _ = try await runner.initialize()
            await loadInfo()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
    
    private func openFolder(at path: String) {
        if let url = URL(string: "file://" + path) {
            NSWorkspace.shared.open(url)
        }
    }
}

// MARK: - Directory Card View
private struct DirectoryCard: View {
    let title: String
    let path: String
    let icon: String
    
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Image(systemName: icon)
                    .foregroundColor(Theme.goldPrimary)
                    .font(.title2)
                Spacer()
                Button("Reveal") {
                    if let url = URL(string: "file://" + path) {
                        NSWorkspace.shared.open(url)
                    }
                }
                .buttonStyle(SymairaSecondaryButtonStyle())
                .controlSize(.small)
            }
            
            Text(title)
                .font(.headline.weight(.semibold))
            
            Text(path)
                .font(.caption.monospaced())
                .foregroundColor(Theme.textSecondary)
                .lineLimit(2)
                .truncationMode(.middle)
        }
        .padding(18)
        .glassmorphicPanel()
    }
}

// MARK: - Target Path Row View
private struct TargetPathRow: View {
    let target: TargetPath
    
    var body: some View {
        HStack(spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text(targetNameFor(target.target))
                    .font(.headline)
                Text(target.target)
                    .font(.caption.monospaced())
                    .foregroundColor(Theme.goldPrimary)
            }
            Spacer()
            Text(target.user)
                .font(.caption.monospaced())
                .foregroundColor(Theme.textSecondary)
                .lineLimit(1)
                .truncationMode(.middle)
            
            Button {
                let folder = (target.user as NSString).deletingLastPathComponent
                if let url = URL(string: "file://" + folder) {
                    NSWorkspace.shared.open(url)
                }
            } label: {
                Image(systemName: "folder")
            }
            .buttonStyle(SymairaSecondaryButtonStyle())
            .controlSize(.small)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
    }
    
    private func targetNameFor(_ target: String) -> String {
        switch target {
        case "opencode": return "OpenCode Harness"
        case "claude": return "Claude Code"
        case "codex": return "Codex Agent"
        case "hermes": return "Hermes System"
        default: return target.capitalized
        }
    }
}
