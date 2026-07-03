import Foundation
import Observation

@Observable
@MainActor
public final class CLICommandRunner {
    public private(set) var logs: [String] = []
    public private(set) var isRunning: Bool = false
    
    private let maxLogs = 1000
    
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
    
    /// Locates the bundled `symskills` binary.
    private func locateBinary() -> URL? {
        if let bundleURL = Bundle.main.url(forResource: "symskills", withExtension: nil) {
            return bundleURL
        }
        
        // Development fallback
        let projectRoot = URL(fileURLWithPath: "/Users/daniel/Dev/Symaira Dev/symaira-skills")
        let devBinary = projectRoot.appendingPathComponent("symskills")
        if FileManager.default.fileExists(atPath: devBinary.path) {
            return devBinary
        }
        
        return nil
    }
    
    /// Core subprocess runner.
    private func runSubprocess(args: [String]) async throws -> Data {
        guard let binaryURL = locateBinary() else {
            let errorMsg = "symskills binary not found in app bundle Resources or project path"
            appendLog("ERROR: \(errorMsg)")
            throw NSError(domain: "SymskillsRunner", code: 404, userInfo: [NSLocalizedDescriptionKey: errorMsg])
        }
        
        isRunning = true
        defer { isRunning = false }
        
        let commandStr = "symskills " + args.joined(separator: " ")
        appendLog("Running: \(commandStr)")
        
        let proc = Process()
        proc.executableURL = binaryURL
        proc.arguments = args
        
        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        proc.standardOutput = stdoutPipe
        proc.standardError = stderrPipe
        
        // Add environment paths for helper commands if needed
        var env = ProcessInfo.processInfo.environment
        if let path = env["PATH"] {
            env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:\(path)"
        } else {
            env["PATH"] = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin"
        }
        proc.environment = env
        
        let outFH = stdoutPipe.fileHandleForReading
        let errFH = stderrPipe.fileHandleForReading
        
        let stdoutTask = Task {
            return (try? outFH.readToEnd()) ?? Data()
        }
        let stderrTask = Task {
            return (try? errFH.readToEnd()) ?? Data()
        }
        
        do {
            try proc.run()
        } catch {
            let _ = await stdoutTask.value
            let _ = await stderrTask.value
            self.appendLog("Failed to run process: \(error.localizedDescription)")
            throw error
        }
        
        proc.waitUntilExit()
        
        let outData = await stdoutTask.value
        let errData = await stderrTask.value
        
        // Log stderr if there is any
        if !errData.isEmpty, let text = String(data: errData, encoding: .utf8) {
            for line in text.components(separatedBy: .newlines) {
                let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
                if !trimmed.isEmpty {
                    self.appendLog("[stderr] \(trimmed)")
                }
            }
        }
        
        let exitCode = proc.terminationStatus
        if exitCode != 0 {
            let errText = String(data: errData, encoding: .utf8) ?? ""
            let cleanErr = errText.trimmingCharacters(in: .whitespacesAndNewlines)
            self.appendLog("Process exited with code \(exitCode): \(cleanErr)")
            throw NSError(domain: "SymskillsRunner", code: Int(exitCode), userInfo: [NSLocalizedDescriptionKey: cleanErr.isEmpty ? "Process exited with code \(exitCode)" : cleanErr])
        } else {
            self.appendLog("Completed successfully.")
            return outData
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
        let proc = Process()
        proc.executableURL = binaryURL
        proc.arguments = ["validate", path, "--json"]
        let pipe = Pipe()
        proc.standardOutput = pipe
        proc.standardError = Pipe()
        
        return try await withCheckedThrowingContinuation { continuation in
            proc.terminationHandler = { process in
                let data = pipe.fileHandleForReading.readDataToEndOfFile()
                struct ValidateResult: Codable {
                    let valid: Bool
                    let issues: [Issue]
                }
                if let res = try? JSONDecoder().decode(ValidateResult.self, from: data) {
                    continuation.resume(returning: res.issues)
                } else {
                    continuation.resume(returning: [])
                }
            }
            do {
                try proc.run()
            } catch {
                continuation.resume(throwing: error)
            }
        }
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
