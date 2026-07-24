import SwiftUI

struct LibraryView: View {
    let runner: CLICommandRunner
    
    @State private var result: SkillListResult?
    @State private var errorMessage: String?
    @State private var importError: String?
    @State private var selectedSkill: SkillSummary?
    @State private var searchText: String = ""
    
    var body: some View {
        HStack(spacing: 24) {
            // Main left list panel
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("PORTABLE BUNDLES")
                            .font(.caption2.monospaced())
                            .tracking(2)
                            .foregroundColor(Theme.goldPrimary)
                        Text("Skill Library")
                            .font(.title2.bold())
                    }
                    Spacer()
                    
                    Button("Import Skill…") {
                        triggerImportPanel()
                    }
                    .buttonStyle(SymairaPrimaryButtonStyle())
                }
                
                // Search bar
                HStack {
                    Image(systemName: "magnifyingglass")
                        .foregroundColor(Theme.textSecondary)
                    TextField("Search library…", text: $searchText)
                        .textFieldStyle(.plain)
                        .foregroundColor(Theme.textPrimary)
                }
                .padding(10)
                .background(Color.white.opacity(0.04))
                .cornerRadius(8)
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .stroke(Theme.borderGlass, lineWidth: 1)
                )
                
                if let result = result {
                    if filteredSkills.isEmpty {
                        ContentUnavailableView("No Skills Found", systemImage: "square.grid.3x3.slash.fill")
                            .frame(maxWidth: .infinity, maxHeight: .infinity)
                    } else {
                        List(filteredSkills, selection: $selectedSkill) { skill in
                            SkillListRow(skill: skill, isSelected: selectedSkill?.id == skill.id)
                                .listRowInsets(EdgeInsets(top: 4, leading: 0, bottom: 4, trailing: 0))
                                .listRowBackground(Color.clear)
                                .onTapGesture {
                                    selectedSkill = skill
                                }
                        }
                        .listStyle(.plain)
                    }
                    
                    // Show global library issues if any
                    if !result.issues.isEmpty {
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Image(systemName: "exclamationmark.triangle.fill")
                                    .foregroundColor(.orange)
                                Text("Library Configuration Issues (\(result.issues.count))")
                                    .font(.headline)
                            }
                            
                            ScrollView {
                                VStack(alignment: .leading, spacing: 6) {
                                    ForEach(result.issues) { issue in
                                        HStack(alignment: .top) {
                                            Text("•")
                                                .foregroundColor(.orange)
                                            VStack(alignment: .leading, spacing: 2) {
                                                Text(issue.message)
                                                    .font(.caption)
                                                    .foregroundColor(Theme.textPrimary)
                                                if let path = issue.path {
                                                    Text(path)
                                                        .font(.caption2.monospaced())
                                                        .foregroundColor(Theme.textMuted)
                                                        .lineLimit(1)
                                                        .truncationMode(.middle)
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                            .frame(maxHeight: 100)
                        }
                        .padding(14)
                        .glassmorphicPanel()
                    }
                } else if errorMessage != nil {
                    ContentUnavailableView {
                        Label("Failed to load skills", systemImage: "exclamationmark.triangle")
                    } description: {
                        Text(errorMessage ?? "")
                    } actions: {
                        Button("Retry") { Task { await loadSkills() } }
                            .buttonStyle(SymairaSecondaryButtonStyle())
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    ProgressView()
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .frame(width: 320)
            
            // Details Panel (Right side)
            VStack {
                if let skill = selectedSkill {
                    SkillDetailView(runner: runner, skill: skill)
                        .id(skill.path) // Forces redraw on select change
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "square.grid.3x3")
                            .font(.system(size: 40))
                            .foregroundColor(Theme.textMuted)
                        Text("Select a skill to inspect, render, and install target-specific variants.")
                            .font(.subheadline)
                            .foregroundColor(Theme.textSecondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 40)
                    }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .glassmorphicPanel()
                }
            }
        }
        .task {
            await loadSkills()
        }
        .alert("Import Failed", isPresented: Binding(
            get: { importError != nil },
            set: { if !$0 { importError = nil } }
        )) {
            Button("OK") { importError = nil }
        } message: {
            Text(importError ?? "")
        }
    }
    
    private var filteredSkills: [SkillSummary] {
        guard let result = result else { return [] }
        if searchText.isEmpty {
            return result.skills
        }
        return result.skills.filter {
            $0.name.localizedCaseInsensitiveContains(searchText) ||
            $0.description.localizedCaseInsensitiveContains(searchText)
        }
    }
    
    private func loadSkills() async {
        do {
            errorMessage = nil
            result = try await runner.list()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
    
    private func triggerImportPanel() {
        let panel = NSOpenPanel()
        panel.title = "Import Skill Directory"
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.canCreateDirectories = false
        panel.allowsMultipleSelection = false
        
        panel.begin { response in
            guard response == .OK, let url = panel.url else { return }
            Task {
                do {
                    let _ = try await runner.importSkill(path: url.path)
                    await loadSkills()
                } catch {
                    // Dedicated surface: errorMessage is only rendered before the
                    // first successful load and would be invisible once skills exist.
                    importError = error.localizedDescription
                }
            }
        }
    }
}

// MARK: - Row View
private struct SkillListRow: View {
    let skill: SkillSummary
    let isSelected: Bool
    
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(skill.name)
                    .font(.headline.weight(.semibold))
                    .foregroundColor(isSelected ? Theme.goldPrimary : Theme.textPrimary)
                Spacer()
                Image(systemName: "chevron.right")
                    .foregroundColor(Theme.textMuted)
                    .font(.caption2)
            }
            
            Text(skill.description)
                .font(.subheadline)
                .foregroundColor(Theme.textSecondary)
                .lineLimit(2)
            
            Text(skill.path)
                .font(.caption2.monospaced())
                .foregroundColor(Theme.textMuted)
                .lineLimit(1)
                .truncationMode(.middle)
        }
        .padding(12)
        .background(isSelected ? Theme.bgCardHover : Color.white.opacity(0.02))
        .cornerRadius(8)
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .stroke(isSelected ? Theme.goldPrimary.opacity(0.3) : Theme.borderGlass, lineWidth: 1)
        )
    }
}
