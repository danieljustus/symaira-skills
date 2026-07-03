import Foundation
import Observation
import SymairaCLIRunner
import SymairaToolKit

@Observable
@MainActor
public final class CLICommandRunner {
    public private(set) var logs: [String] = []
    public private(set) var isRunning: Bool = false

    private let maxLogs = 1000

    private let runner = CLIRunner(defaultTimeout: 120)
    private let locator: BinaryLocator = {
        // Repo root (../symskills) as last resort keeps the pre-AppKit dev
        // workflow working when running from Xcode without a bundled binary.
        let projectRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent() // SymskillsKit/
            .deletingLastPathComponent() // Sources/
            .deletingLastPathComponent() // client/
            .deletingLastPathComponent() // repo root
        return BinaryLocator(extraDirectories: ["/opt/homebrew/bin", "/usr/local/bin", projectRoot.path])
    }()

    public init() {}

    public func clearLogs() {
        logs.removeAll()
    }

    public func appendLog(_ message: String) {
        let timestamp = ISO8601DateFormatter.string(from: Date(), timeZone: .current, formatOptions: [.withTime, .withColonSeparatorInTime])
        logs.append("[\(timestamp)] \(message)")
        if logs.count > maxLogs {
            logs.removeFirst(logs.count - maxLogs)
        }
    }

    private func locateBinary() -> URL? {
        locator.locate("symskills")?.url
    }

    /// Core subprocess runner.
    private func runSubprocess(args: [String]) async throws -> Data {
        guard let binaryURL = locateBinary() else {
            let errorMsg = "symskills binary not found. Install it via 'brew install danieljustus/tap/symskills' or build it first ('make build')."
            appendLog("ERROR: \(errorMsg)")
            throw NSError(domain: "SymskillsRunner", code: 404, userInfo: [NSLocalizedDescriptionKey: errorMsg])
        }

        isRunning = true
        defer { isRunning = false }

        let commandStr = "symskills " + args.joined(separator: " ")
        appendLog("Running: \(commandStr)")

        let result: CLIResult
        do {
            result = try await runner.run(binaryURL, arguments: args)
        } catch {
            appendLog("Failed to run process: \(error.localizedDescription)")
            throw error
        }

        // Log stderr if there is any
        if !result.stderr.isEmpty, let text = String(data: result.stderr, encoding: .utf8) {
            for line in text.components(separatedBy: .newlines) {
                let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
                if !trimmed.isEmpty {
                    appendLog("[stderr] \(trimmed)")
                }
            }
        }

        if result.exitCode != 0 {
            let cleanErr = result.stderrText
            appendLog("Process exited with code \(result.exitCode): \(cleanErr)")
            throw NSError(domain: "SymskillsRunner", code: Int(result.exitCode), userInfo: [NSLocalizedDescriptionKey: cleanErr.isEmpty ? "Process exited with code \(result.exitCode)" : cleanErr])
        } else {
            appendLog("Completed successfully.")
            return result.stdout
        }
    }
    
    // MARK: - Command API
    
    public func doctor() async throws -> DoctorInfo {
        let data = try await runSubprocess(args: ["doctor", "--json"])
        return try JSONDecoder().decode(DoctorInfo.self, from: data)
    }
    
    public func list() async throws -> SkillListResult {
        let data = try await runSubprocess(args: ["list", "--json"])
        return try JSONDecoder().decode(SkillListResult.self, from: data)
    }
    
    public func initialize() async throws -> String {
        let data = try await runSubprocess(args: ["init", "--force"])
        return String(data: data, encoding: .utf8) ?? "Done"
    }
    
    public func importSkill(path: String) async throws -> ImportResult {
        let data = try await runSubprocess(args: ["import", path, "--json"])
        // ImportResult is defined in internal/skill/skill.go
        struct ImportRes: Codable {
            let name: String
            let path: String
        }
        let res = try JSONDecoder().decode(ImportRes.self, from: data)
        return ImportResult(name: res.name, path: res.path)
    }
    
    public func validate(path: String) async throws -> Bool {
        // Runs validate. If validation fails, process exit code is non-zero (ExitData).
        // If it returns ExitData, runSubprocess throws. We can handle it.
        do {
            let _ = try await runSubprocess(args: ["validate", path, "--json"])
            return true
        } catch {
            return false
        }
    }
    
    public func getIssues(path: String) async throws -> [Issue] {
        do {
            let data = try await runSubprocess(args: ["validate", path, "--json"])
            struct ValidateResult: Codable {
                let valid: Bool
                let issues: [Issue]
            }
            let res = try JSONDecoder().decode(ValidateResult.self, from: data)
            return res.issues
        } catch {
            // If it failed because of errors, the JSON output is still on stdout. Let's see.
            // When command fails with exit code, our runSubprocess throws. But we can parse issues out of the thrown error message if it is JSON, or we can run without throwing by catching output.
            // Let's modify runSubprocess or just try running a custom command for validation.
            // Actually, we can run validate with --json and if it fails, parse the stdout output.
            // Let's implement a specific validation parser.
            return try await runValidationSubprocess(path: path)
        }
    }
    
    private func runValidationSubprocess(path: String) async throws -> [Issue] {
        guard let binaryURL = locateBinary() else {
            throw NSError(domain: "SymskillsRunner", code: 404, userInfo: [NSLocalizedDescriptionKey: "symskills not found"])
        }
        // Validation failures exit non-zero but still print the issue JSON
        // on stdout, so decode regardless of the exit code.
        let result = try await runner.run(binaryURL, arguments: ["validate", path, "--json"])
        struct ValidateResult: Codable {
            let valid: Bool
            let issues: [Issue]
        }
        if let res = try? JSONDecoder().decode(ValidateResult.self, from: result.stdout) {
            return res.issues
        }
        return []
    }
    
    public func inspect(path: String) async throws -> SkillBundle {
        let data = try await runSubprocess(args: ["inspect", path, "--json"])
        return try JSONDecoder().decode(SkillBundle.self, from: data)
    }
    
    public func render(path: String, target: String = "all") async throws -> [Rendered] {
        let data = try await runSubprocess(args: ["render", path, "--target", target, "--json"])
        return try JSONDecoder().decode([Rendered].self, from: data)
    }
    
    public func diff(path: String, target: String) async throws -> [Change] {
        let data = try await runSubprocess(args: ["diff", path, "--target", target, "--json"])
        return try JSONDecoder().decode([Change].self, from: data)
    }
    
    public func install(path: String, target: String, scope: String = "user", mode: String = "symlink", dryRun: Bool = false) async throws -> InstallResult {
        var args = ["install", path, "--target", target, "--scope", scope, "--mode", mode, "--json"]
        if dryRun {
            args.append("--dry-run")
        }
        let data = try await runSubprocess(args: args)
        return try JSONDecoder().decode(InstallResult.self, from: data)
    }
    
    public func uninstall(name: String, target: String, scope: String = "user") async throws -> String {
        let data = try await runSubprocess(args: ["uninstall", name, "--target", target, "--scope", scope])
        return String(data: data, encoding: .utf8) ?? "Uninstalled successfully."
    }
}
