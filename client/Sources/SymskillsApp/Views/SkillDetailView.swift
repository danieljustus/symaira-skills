import SwiftUI

struct SkillDetailView: View {
    let runner: CLICommandRunner
    let skill: SkillSummary
    
    @State private var bundle: SkillBundle?
    @State private var issues: [Issue] = []
    @State private var errorMessage: String?
    @State private var diffPresentation: DiffPresentation?
    @State private var renderedPreview: Rendered?
    
    // Installation configuration per target
    @State private var installMode: String = "symlink"
    @State private var installScope: String = "user"
    
    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            if let bundle = bundle {
                ScrollView {
                    VStack(alignment: .leading, spacing: 24) {
                        
                        // Header Box
                        VStack(alignment: .leading, spacing: 12) {
                            HStack {
                                Text(bundle.frontmatter.name)
                                    .font(.title.bold())
                                    .foregroundColor(Theme.goldPrimary)
                                Spacer()
                                if let ver = bundle.frontmatter.version {
                                    Text("v\(ver)")
                                        .font(.headline.monospaced())
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 4)
                                        .background(Theme.goldPrimary.opacity(0.15))
                                        .foregroundColor(Theme.goldPrimary)
                                        .cornerRadius(6)
                                }
                            }
                            
                            Text(bundle.frontmatter.description)
                                .font(.body)
                                .foregroundColor(Theme.textPrimary)
                            
                            Divider()
                                .background(Theme.borderGlass)
                            
                            // Meta Grid
                            LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 10) {
                                MetaRow(label: "Author", value: bundle.frontmatter.author ?? "Unknown")
                                MetaRow(label: "License", value: bundle.frontmatter.license ?? "Unknown")
                                MetaRow(label: "Source", value: bundle.manifest.skill.source.isEmpty ? "Local" : bundle.manifest.skill.source)
                                MetaRow(label: "Path", value: bundle.root)
                            }
                        }
                        .padding(20)
                        .glassmorphicPanel()
                        
                        // Validation issues box (specific to this skill)
                        if !issues.isEmpty {
                            VStack(alignment: .leading, spacing: 12) {
                                Label("Validation Warnings & Errors", systemImage: "exclamationmark.triangle.fill")
                                    .foregroundColor(.orange)
                                    .font(.headline)
                                
                                ForEach(issues) { issue in
                                    HStack(alignment: .top) {
                                        Text(issue.severity.uppercased())
                                            .font(.caption2.bold().monospaced())
                                            .padding(.horizontal, 6)
                                            .padding(.vertical, 2)
                                            .background(issue.severity.lowercased() == "error" ? Color.red.opacity(0.15) : Color.orange.opacity(0.15))
                                            .foregroundColor(issue.severity.lowercased() == "error" ? .red : .orange)
                                            .cornerRadius(4)
                                        
                                        VStack(alignment: .leading, spacing: 2) {
                                            Text(issue.message)
                                                .font(.caption)
                                            if let p = issue.path {
                                                Text(p)
                                                    .font(.caption2.monospaced())
                                                    .foregroundColor(Theme.textMuted)
                                            }
                                        }
                                    }
                                }
                            }
                            .padding(18)
                            .glassmorphicPanel()
                        }
                        
                        // Install settings control
                        VStack(alignment: .leading, spacing: 12) {
                            Text("INSTALLATION PARAMETERS")
                                .font(.caption.bold().monospaced())
                                .foregroundColor(Theme.goldPrimary)
                            
                            HStack(spacing: 24) {
                                Picker("Mode:", selection: $installMode) {
                                    Text("Symlink").tag("symlink")
                                    Text("Copy").tag("copy")
                                }
                                .pickerStyle(.radioGroup)
                                .horizontalRadio()
                                
                                Picker("Scope:", selection: $installScope) {
                                    Text("User").tag("user")
                                    Text("Project").tag("project")
                                }
                                .pickerStyle(.radioGroup)
                                .horizontalRadio()
                            }
                        }
                        .padding(16)
                        .glassmorphicPanel(addCorners: false)
                        
                        // Targets rendering & install
                        Text("HARNESS TARGETS")
                            .font(.caption.bold().monospaced())
                            .foregroundColor(Theme.goldPrimary)
                            .padding(.top, 8)
                        
                        VStack(spacing: 16) {
                            TargetRow(runner: runner, path: skill.path, target: "opencode", bundle: bundle, mode: installMode, scope: installScope, onDiff: showDiff, onRender: showRender)
                            TargetRow(runner: runner, path: skill.path, target: "claude", bundle: bundle, mode: installMode, scope: installScope, onDiff: showDiff, onRender: showRender)
                            TargetRow(runner: runner, path: skill.path, target: "codex", bundle: bundle, mode: installMode, scope: installScope, onDiff: showDiff, onRender: showRender)
                            TargetRow(runner: runner, path: skill.path, target: "hermes", bundle: bundle, mode: installMode, scope: installScope, onDiff: showDiff, onRender: showRender)
                        }
                    }
                }
            } else if errorMessage != nil {
                ContentUnavailableView("Failed to load details", systemImage: "exclamationmark.triangle")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                ProgressView("Loading skill details…")
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task {
            await loadDetails()
        }
        .sheet(item: $diffPresentation) { presentation in
            DiffView(target: presentation.targetName, changes: presentation.changes)
                .frame(minWidth: 600, minHeight: 450)
        }
        .sheet(item: $renderedPreview) { rendered in
            RenderedPreviewView(rendered: rendered)
                .frame(minWidth: 700, minHeight: 550)
        }
    }
    
    private func loadDetails() async {
        do {
            errorMessage = nil
            bundle = try await runner.inspect(path: skill.path)
            issues = try await runner.getIssues(path: skill.path)
        } catch {
            errorMessage = error.localizedDescription
        }
    }
    
    private func showDiff(targetName: String, changes: [Change]) {
        // Set target name and changes atomically so the sheet never renders
        // with a stale or empty title.
        self.diffPresentation = DiffPresentation(targetName: targetName, changes: changes)
    }
    
    private func showRender(rendered: Rendered) {
        self.renderedPreview = rendered
    }
}

// MARK: - Diff sheet presentation model
/// Carries the diff target display name and the changes atomically, so the
/// sheet item always provides both values together.
private struct DiffPresentation: Identifiable {
    let targetName: String
    let changes: [Change]

    var id: String {
        targetName + ":" + changes.map(\.path).joined(separator: ",")
    }
}

// MARK: - Meta Row helper
private struct MetaRow: View {
    let label: String
    let value: String
    
    var body: some View {
        HStack(alignment: .top) {
            Text("\(label):")
                .foregroundColor(Theme.textSecondary)
                .font(.subheadline)
                .frame(width: 80, alignment: .leading)
            Text(value)
                .foregroundColor(Theme.textPrimary)
                .font(.subheadline.monospaced())
                .lineLimit(1)
                .truncationMode(.middle)
            Spacer()
        }
    }
}

// MARK: - Picker layout helpers
extension View {
    func horizontalRadio() -> some View {
        self.labelsHidden()
            .controlSize(.small)
            .offset(x: -8)
    }
}

// MARK: - Target Row UI Component
private struct TargetRow: View {
    let runner: CLICommandRunner
    let path: String
    let target: String
    let bundle: SkillBundle
    let mode: String
    let scope: String
    let onDiff: (String, [Change]) -> Void
    let onRender: (Rendered) -> Void
    
    @State private var isHarnessEnabled: Bool = true
    @State private var actionMessage: String?
    @State private var isError: Bool = false
    
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text(targetNameFor(target))
                        .font(.headline)
                    HStack(spacing: 8) {
                        Text(target)
                            .font(.caption2.monospaced())
                            .foregroundColor(Theme.goldPrimary)
                        
                        Circle()
                            .fill(isHarnessEnabled ? Color.green : Color.secondary)
                            .frame(width: 6, height: 6)
                        Text(isHarnessEnabled ? "Enabled in Config" : "Disabled")
                            .font(.caption2)
                            .foregroundColor(Theme.textSecondary)
                    }
                }
                Spacer()
                
                // Commands buttons
                HStack(spacing: 8) {
                    Button("Preview") {
                        Task { await previewTarget() }
                    }
                    .buttonStyle(SymairaSecondaryButtonStyle())
                    .controlSize(.small)
                    .disabled(!isHarnessEnabled)
                    
                    Button("Diff") {
                        Task { await diffTarget() }
                    }
                    .buttonStyle(SymairaSecondaryButtonStyle())
                    .controlSize(.small)
                    .disabled(!isHarnessEnabled)
                    
                    Button("Uninstall") {
                        Task { await uninstallTarget() }
                    }
                    .buttonStyle(SymairaSecondaryButtonStyle())
                    .controlSize(.small)
                    .disabled(!isHarnessEnabled)
                    
                    Button("Install") {
                        Task { await installTarget() }
                    }
                    .buttonStyle(SymairaPrimaryButtonStyle())
                    .controlSize(.small)
                    .disabled(!isHarnessEnabled)
                }
            }
            
            if let msg = actionMessage {
                Text(msg)
                    .font(.caption2.monospaced())
                    .foregroundColor(isError ? .red : Theme.goldSecondary)
                    .padding(8)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(isError ? Color.red.opacity(0.08) : Color.white.opacity(0.04))
                    .cornerRadius(6)
                    .overlay(
                        RoundedRectangle(cornerRadius: 6)
                            .stroke(isError ? Color.red.opacity(0.2) : Theme.borderGlass, lineWidth: 1)
                    )
            }
        }
        .padding(16)
        .glassmorphicPanel(addCorners: false)
        .task {
            // Check if this target is explicitly configured in manifest
            if let targetCfg = bundle.manifest.targets?[target] {
                isHarnessEnabled = targetCfg.enabled
            } else {
                isHarnessEnabled = true // default to enabled if not config override
            }
        }
    }
    
    private func targetNameFor(_ target: String) -> String {
        switch target {
        case "opencode": return "OpenCode"
        case "claude": return "Claude Code"
        case "codex": return "Codex"
        case "hermes": return "Hermes"
        default: return target.capitalized
        }
    }
    
    private func previewTarget() async {
        do {
            actionMessage = nil
            isError = false
            let rendered = try await runner.render(path: path, target: target)
            if let first = rendered.first {
                onRender(first)
            }
        } catch {
            isError = true
            actionMessage = "Render failed: \(error.localizedDescription)"
        }
    }
    
    private func diffTarget() async {
        do {
            actionMessage = nil
            isError = false
            let changes = try await runner.diff(path: path, target: target)
            onDiff(targetNameFor(target), changes)
        } catch {
            isError = true
            actionMessage = "Diff failed: \(error.localizedDescription)"
        }
    }
    
    private func installTarget() async {
        do {
            actionMessage = nil
            isError = false
            let res = try await runner.install(path: path, target: target, scope: scope, mode: mode)
            actionMessage = "Installed: \(res.name) at \(res.path)"
        } catch {
            isError = true
            actionMessage = "Install failed: \(error.localizedDescription)"
        }
    }
    
    private func uninstallTarget() async {
        do {
            actionMessage = nil
            isError = false
            let res = try await runner.uninstall(name: bundle.frontmatter.name, target: target, scope: scope)
            actionMessage = res
        } catch {
            isError = true
            actionMessage = "Uninstall failed: \(error.localizedDescription)"
        }
    }
}

// MARK: - Render Preview Sheet
private struct RenderedPreviewView: View {
    let rendered: Rendered
    @Environment(\.dismiss) private var dismiss
    
    var body: some View {
        NavigationStack {
            ZStack {
                Theme.bgDark.ignoresSafeArea()
                BlueprintGrid()
                
                VStack(alignment: .leading, spacing: 16) {
                    HStack {
                        Text("TARGET:")
                            .foregroundColor(Theme.textSecondary)
                        Text(rendered.target.uppercased())
                            .foregroundColor(Theme.goldPrimary)
                            .font(.headline.monospaced())
                        
                        Spacer()
                        
                        Text("RENDERED NAME:")
                            .foregroundColor(Theme.textSecondary)
                        Text(rendered.name)
                            .foregroundColor(Theme.goldPrimary)
                            .font(.headline)
                    }
                    .padding(12)
                    .glassmorphicPanel(addCorners: false)
                    
                    TextEditor(text: .constant(rendered.skillMd ?? ""))
                        .font(.system(.body, design: .monospaced))
                        .foregroundColor(Theme.textPrimary)
                        .scrollContentBackground(.hidden)
                        .background(Theme.bgCard)
                        .cornerRadius(8)
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(Theme.borderGlass, lineWidth: 1)
                        )
                }
                .padding(24)
            }
            .navigationTitle("Render Preview")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
        }
    }
}
