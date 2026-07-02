namespace Shell1448.App;

using System.Windows;
using System.Windows.Controls;

/// <summary>Модальный ввод пароля для аварийного выхода — без зависимости от VisualBasic.</summary>
public static class PasswordPrompt
{
    public static string Ask(Window owner)
    {
        var box = new PasswordBox { Margin = new Thickness(0, 8, 0, 12), FontSize = 16 };
        var ok = new Button { Content = "Выйти", IsDefault = true, Padding = new Thickness(16, 6, 16, 6) };
        var panel = new StackPanel { Margin = new Thickness(20) };
        panel.Children.Add(new TextBlock { Text = "Пароль для выхода из киоска:", FontSize = 14 });
        panel.Children.Add(box);
        panel.Children.Add(ok);

        var dlg = new Window
        {
            Title = "14:48 Shell",
            Content = panel,
            SizeToContent = SizeToContent.WidthAndHeight,
            WindowStartupLocation = WindowStartupLocation.CenterOwner,
            Owner = owner,
            ResizeMode = ResizeMode.NoResize,
            WindowStyle = WindowStyle.ToolWindow,
            Topmost = true,
        };
        ok.Click += (_, _) => dlg.DialogResult = true;
        box.Loaded += (_, _) => box.Focus();
        return dlg.ShowDialog() == true ? box.Password : "";
    }
}
