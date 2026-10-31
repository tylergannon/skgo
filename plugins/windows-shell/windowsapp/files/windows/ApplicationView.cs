using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace SKGoShell;

// This is the application's native UI slot. Add generated NativeAPI calls here
// using a separate HttpClient; no application credentials belong in WebView code.
public sealed class ApplicationView : Grid, IDisposable
{
    private ApplicationWebView? browser;
    private readonly ApplicationServer server = new();
    private readonly CancellationTokenSource lifetime = new();
    private readonly TextBlock status = new() { Text = "Starting application…", Margin = new Thickness(20) };
    private bool started, disposed;
    public ApplicationView()
    {
        RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        RowDefinitions.Add(new RowDefinition());
        Children.Add(status);
        server.Exited += error => DispatcherQueue.TryEnqueue(() => { if (!disposed) status.Text = error; });
        Loaded += async (_, _) => {
            if (started || disposed) return;
            started = true;
            try {
                var configured = Environment.GetEnvironmentVariable("SKGO_PAGE_URL");
                var page = configured is null ? await server.StartAsync(lifetime.Token) : new Uri(configured);
                if (disposed) return;
                status.Text = "SKGo · Windows native shell";
                browser = new ApplicationWebView(new WebOptions(page));
                browser.Failed += error => status.Text = error;
                SetRow(browser, 1); Children.Add(browser);
            } catch (Exception e) { if (!disposed) status.Text = "Application failed: " + e.Message; }
        };
    }
    public void Dispose()
    {
        if (disposed) return; disposed = true;
        lifetime.Cancel(); browser?.Dispose(); server.Dispose(); lifetime.Dispose();
    }
}
