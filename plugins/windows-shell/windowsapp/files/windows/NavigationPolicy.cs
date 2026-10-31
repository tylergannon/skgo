namespace SKGoShell;

// Origin matching never uses host substrings. Navigation and bridge authority
// are independent. An origin grants no permission to execute OS commands.
public sealed class NavigationPolicy
{
    private readonly HashSet<string> navigation;
    private readonly HashSet<string> bridge;
    public NavigationPolicy(Uri initial, IEnumerable<Uri>? allowed = null, IEnumerable<Uri>? trustedMessages = null)
    {
        navigation = new((allowed ?? []).Append(initial).Select(Origin), StringComparer.Ordinal);
        bridge = new((trustedMessages ?? []).Select(Origin), StringComparer.Ordinal);
    }
    public static string Origin(Uri uri)
    {
        if (!uri.IsAbsoluteUri || (uri.Scheme != "https" && uri.Scheme != "http") || uri.UserInfo.Length != 0)
            throw new ArgumentException("Only absolute HTTP(S) URLs without userinfo are supported");
        return $"{uri.Scheme}://{uri.IdnHost.ToLowerInvariant()}:{uri.Port}";
    }
    private static bool Contains(HashSet<string> set, string url)
    {
        try { return set.Contains(Origin(new Uri(url, UriKind.Absolute))); }
        catch (Exception e) when (e is ArgumentException || e is UriFormatException) { return false; }
    }
    public bool CanNavigate(string url) => Contains(navigation, url);
    public bool CanMessage(string source, string currentPage) => Contains(bridge, source) && Contains(bridge, currentPage);
}
