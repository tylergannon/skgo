using Microsoft.UI.Xaml;

namespace SKGoShell;

public partial class App : Application
{
    private Window? window;
    public App() { InitializeComponent(); }
    protected override void OnLaunched(LaunchActivatedEventArgs args)
    {
        var view = new ApplicationView();
        window = new Window { Title = "SKGo Shell", Content = view };
        window.AppWindow.Resize(new Windows.Graphics.SizeInt32(1120, 800));
        window.Closed += (_, _) => view.Dispose();
        window.Activate();
    }
}
