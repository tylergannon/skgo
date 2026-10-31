using Microsoft.UI.Xaml.Controls;
using Microsoft.Web.WebView2.Core;

namespace SKGoShell;

public sealed record WebOptions(Uri InitialPage, Uri[]? AllowedOrigins = null, Uri[]? MessageOrigins = null);
public sealed record WebRequestInfo(string Uri, string Method, CoreWebView2WebResourceContext Context);

// Application-owned hooks run before the first page is loaded. Messages expose
// raw JSON for the application to validate, never a built-in privileged RPC.
public sealed class ApplicationWebView : Grid, IDisposable
{
    private readonly WebView2 view = new();
    private readonly TextBlock error = new() { IsHitTestVisible = false, VerticalAlignment = Microsoft.UI.Xaml.VerticalAlignment.Top };
    private readonly WebOptions options;
    private readonly NavigationPolicy policy;
    private bool started, disposed;
    public event Func<CoreWebView2, Task>? Configure;
    public event Action<string, string>? Message;
    public event Action<string>? NavigationBlocked;
    public event Action<WebRequestInfo>? RequestObserved;
    public event Action<string>? Failed;
    public string? BrowserVersion => view.CoreWebView2?.Environment.BrowserVersionString;
    public ApplicationWebView(WebOptions options)
    {
        this.options = options;
        policy = new(options.InitialPage, options.AllowedOrigins, options.MessageOrigins);
        Children.Add(view); Children.Add(error);
        Loaded += async (_, _) => await StartAsync();
    }
    private async Task StartAsync()
    {
        if (started || disposed) return;
        started = true;
        try {
            await view.EnsureCoreWebView2Async();
            if (disposed) return;
            var core = view.CoreWebView2;
            core.NavigationStarting += (_, e) => {
                if (!policy.CanNavigate(e.Uri)) { e.Cancel = true; NavigationBlocked?.Invoke(e.Uri); }
                else error.Text = "";
            };
            core.NewWindowRequested += (_, e) => {
                e.Handled = true;
                if (policy.CanNavigate(e.Uri)) core.Navigate(e.Uri);
                else NavigationBlocked?.Invoke(e.Uri);
            };
            core.WebMessageReceived += (_, e) => {
                if (policy.CanMessage(e.Source, core.Source)) Message?.Invoke(e.Source, e.WebMessageAsJson);
            };
            core.AddWebResourceRequestedFilter("*", CoreWebView2WebResourceContext.All);
            core.WebResourceRequested += (_, e) => RequestObserved?.Invoke(new(e.Request.Uri, e.Request.Method, e.ResourceContext));
            core.ProcessFailed += (_, e) => ShowError("Browser process failed: " + e.ProcessFailedKind);
            core.NavigationCompleted += (_, e) => {
                if (!e.IsSuccess && e.WebErrorStatus != CoreWebView2WebErrorStatus.OperationCanceled)
                    ShowError("Navigation failed: " + e.WebErrorStatus);
            };
            if (Configure is {} configure) foreach (Func<CoreWebView2, Task> hook in configure.GetInvocationList()) await hook(core);
            if (!disposed) core.Navigate(options.InitialPage.AbsoluteUri);
        } catch (Exception e) { if (!disposed) ShowError(e.Message); }
    }
    private void ShowError(string message) { error.Text = message; Failed?.Invoke(message); }
    public void Navigate(Uri uri)
    {
        if (!policy.CanNavigate(uri.AbsoluteUri)) { NavigationBlocked?.Invoke(uri.AbsoluteUri); return; }
        if (view.CoreWebView2 is null) throw new InvalidOperationException("WebView is not ready");
        error.Text = ""; view.CoreWebView2.Navigate(uri.AbsoluteUri);
    }
    public void Dispose() { if (disposed) return; disposed = true; view.Close(); }
}
