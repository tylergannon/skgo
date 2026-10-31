using System.Diagnostics;
using System.Text;
using System.Text.Json;

namespace SKGoShell;

// The application owns this subprocess. Its redirected stdin is a lifetime
// pipe: even an unexpected UI exit closes it and lets Go shut down.
public sealed class ApplicationServer : IDisposable
{
    private readonly Process process = new();
    private readonly StringBuilder diagnostics = new();
    private bool started, disposed;
    public event Action<string>? Exited;
    public async Task<Uri> StartAsync(CancellationToken cancellationToken)
    {
        if (disposed || started) throw new InvalidOperationException("Application server already started or closed");
        process.StartInfo = new(Path.Combine(AppContext.BaseDirectory, "skgo-application.exe")) {
            WorkingDirectory = AppContext.BaseDirectory,
            UseShellExecute = false, CreateNoWindow = true,
            RedirectStandardInput = true, RedirectStandardOutput = true, RedirectStandardError = true
        };
        process.EnableRaisingEvents = true;
        process.ErrorDataReceived += (_, e) => {
            if (e.Data is not null) lock (diagnostics) {
                diagnostics.AppendLine(e.Data);
                if (diagnostics.Length > 8192) diagnostics.Remove(0, diagnostics.Length - 8192);
            }
        };
        process.Exited += (_, _) => { if (!disposed) Exited?.Invoke("The local application server stopped."); };
        try {
            if (!process.Start()) throw new InvalidOperationException("Cannot start the application server");
            started = true;
            process.BeginErrorReadLine();
            using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
            timeout.CancelAfter(TimeSpan.FromSeconds(30));
            var ready = await process.StandardOutput.ReadLineAsync(timeout.Token);
            if (ready is null) {
                process.WaitForExit(2000);
                lock (diagnostics) throw new InvalidOperationException("Application server failed to start: " + diagnostics);
            }
            using var json = JsonDocument.Parse(ready);
            var origin = new Uri(json.RootElement.GetProperty("origin").GetString()!, UriKind.Absolute);
            if (origin.Scheme != "http" || origin.Host != "127.0.0.1" || origin.UserInfo != "" || origin.AbsolutePath != "/" || origin.Query != "" || origin.Fragment != "")
                throw new InvalidOperationException("Application server returned an invalid loopback origin");
            // Drain later application output without allowing it to block Go.
            _ = DrainOutputAsync(process.StandardOutput);
            return origin;
        } catch { Dispose(); throw; }
    }
    private static async Task DrainOutputAsync(StreamReader output)
    {
        try { while (await output.ReadLineAsync() is not null) { } }
        catch (Exception e) when (e is IOException || e is ObjectDisposedException) { }
    }
    public void Dispose()
    {
        if (disposed) return;
        disposed = true;
        try {
            if (started && !process.HasExited) {
                process.StandardInput.Close();
                if (!process.WaitForExit(3000)) process.Kill(entireProcessTree: true);
            }
        } finally { process.Dispose(); }
    }
}
