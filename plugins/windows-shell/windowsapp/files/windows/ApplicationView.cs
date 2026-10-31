using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace SKGoShell;

// This is the application's native UI slot. Add generated NativeAPI calls here
// using a separate HttpClient; no application credentials belong in WebView code.
public sealed class ApplicationView : Grid, IDisposable
{
    private readonly ApplicationWebView browser;
    public ApplicationView()
    {
        RowDefinitions.Add(new RowDefinition { Height = GridLength.Auto });
        RowDefinitions.Add(new RowDefinition());
        Children.Add(new TextBlock { Text = "SKGo · Windows native shell", FontSize = 24, Margin = new Thickness(20) });
        browser = new ApplicationWebView(new WebOptions(new Uri(Environment.GetEnvironmentVariable("SKGO_PAGE_URL") ?? "http://127.0.0.1:8080/")));
        SetRow(browser, 1);
        Children.Add(browser);
    }
    public void Dispose() => browser.Dispose();
}
