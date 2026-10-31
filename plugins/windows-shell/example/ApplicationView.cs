using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using SKGoNative;
using GeneratedRemote;
using System.Text.Json;

namespace SKGoShell;

// Synthetic sample. Production applications supply their own credentials and
// origins; the browser never receives the HttpClient's authorization header.
public sealed class ApplicationView : Grid, IDisposable
{
    private readonly HttpClient http = new(new HttpClientHandler { AllowAutoRedirect = false });
    private readonly RemoteClient client;
    private readonly NativeAPI api;
    private readonly ApplicationWebView browser;
    private readonly CancellationTokenSource lifetime = new();
    private readonly TextBlock status = new() { Text = "Connecting to native API…" };
    private readonly TextBlock message = new() { Text = "Web event: waiting" };
    private readonly TextBlock navigation = new() { Text = "Navigation: initial page" };
    private readonly TextBlock engine = new() { Text = "WebView2: starting" };
    private readonly TextBox input = new() { Text = "Windows native command", Width = 320 };
    private readonly Button save = new() { Content = "Save through Go", IsEnabled = false };
    private bool disposed;
    private int messages;
    public ApplicationView()
    {
        RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        RowDefinitions.Add(new RowDefinition());
        var controls = new StackPanel { Spacing = 10, Margin = new Thickness(20) };
        controls.Children.Add(new TextBlock { Text = "SKGo · Native Windows + Go", FontSize = 26 });
        var row = new StackPanel { Orientation = Orientation.Horizontal, Spacing = 12 };
        row.Children.Add(input); row.Children.Add(save); controls.Children.Add(row);
        foreach (var label in new[] { status, message, navigation, engine }) controls.Children.Add(label);
        AutomationProperties.SetAutomationId(input, "NativeInput");
        AutomationProperties.SetAutomationId(save, "NativeSave");
        AutomationProperties.SetAutomationId(status, "NativeStatus");
        AutomationProperties.SetAutomationId(message, "WebMessage");
        Children.Add(controls);

        http.DefaultRequestHeaders.Authorization = new("Bearer", Environment.GetEnvironmentVariable("SKGO_API_TOKEN") ?? "synthetic-native-token");
        client = new(Environment.GetEnvironmentVariable("SKGO_API_URL") ?? "http://127.0.0.1:48321", new HttpClientTransport(http));
        api = new(client);
        var page = new Uri(Environment.GetEnvironmentVariable("SKGO_PAGE_URL") ?? "http://127.0.0.1:48322/");
        var otherPage = new Uri(Environment.GetEnvironmentVariable("SKGO_OTHER_PAGE_URL") ?? "http://localhost:48322/");
        browser = new(new WebOptions(page, AllowedOrigins: [otherPage], MessageOrigins: [page]));
        browser.Configure += async core => {
            engine.Text = "WebView2: " + core.Environment.BrowserVersionString;
            await core.AddScriptToExecuteOnDocumentCreatedAsync("document.addEventListener('DOMContentLoaded',()=>{document.getElementById('extension')?.replaceChildren('Application document script is running.');chrome.webview.postMessage({kind:'example',text:'Synthetic web event'});});");
        };
        browser.Message += (_, json) => {
            using var doc = JsonDocument.Parse(json);
            if (doc.RootElement.TryGetProperty("kind", out var kind) && kind.GetString() == "example" && doc.RootElement.TryGetProperty("text", out var text))
                message.Text = $"Web event {++messages}: " + text.GetString();
        };
        browser.NavigationBlocked += uri => navigation.Text = "Blocked: " + uri;
        browser.Failed += error => navigation.Text = error;
        SetRow(browser, 1); Children.Add(browser);
        Loaded += async (_, _) => await ReadAsync();
        save.Click += async (_, _) => {
            save.IsEnabled = false;
            try {
                await api.setValueAsync(input.Text, cancellationToken: lifetime.Token);
                status.Text = "Confirmed: " + await api.getValueAsync(cancellationToken: lifetime.Token);
            } catch (Exception e) { if (!disposed) status.Text = "Native error: " + e.Message; }
            finally { if (!disposed) save.IsEnabled = true; }
        };
    }
    private async Task ReadAsync()
    {
        try { status.Text = "Read: " + await api.getValueAsync(cancellationToken: lifetime.Token); save.IsEnabled = true; }
        catch (Exception e) { if (!disposed) status.Text = "Native error: " + e.Message; }
    }
    public void Dispose()
    {
        if (disposed) return; disposed = true;
        lifetime.Cancel(); client.Dispose(); http.Dispose(); browser.Dispose(); lifetime.Dispose();
    }
}
