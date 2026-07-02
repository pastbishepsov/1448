namespace Shell1448.App;

using System.Runtime.InteropServices;

/// <summary>
/// Низкоуровневый клавиатурный хук: в киоск-режиме глотает Win, Alt+Tab, Alt+F4,
/// Alt+Esc, Ctrl+Esc. Ctrl+Alt+Del перехватить нельзя (Secure Attention Sequence) —
/// его отключает политика/Shell Launcher в фазе 2.
/// Аварийная комбинация Ctrl+Alt+Shift+Q пропускается наружу (обрабатывает MainWindow).
/// </summary>
public sealed class KeyboardHook : IDisposable
{
    private const int WH_KEYBOARD_LL = 13;
    private const int WM_KEYDOWN = 0x0100, WM_SYSKEYDOWN = 0x0104;
    private const int VK_TAB = 0x09, VK_ESC = 0x1B, VK_F4 = 0x73,
                      VK_LWIN = 0x5B, VK_RWIN = 0x5C, VK_Q = 0x51;

    private delegate IntPtr HookProc(int nCode, IntPtr wParam, IntPtr lParam);

    [DllImport("user32.dll")] private static extern IntPtr SetWindowsHookExW(int idHook, HookProc lpfn, IntPtr hMod, uint dwThreadId);
    [DllImport("user32.dll")] private static extern bool UnhookWindowsHookEx(IntPtr hhk);
    [DllImport("user32.dll")] private static extern IntPtr CallNextHookEx(IntPtr hhk, int nCode, IntPtr wParam, IntPtr lParam);
    [DllImport("user32.dll")] private static extern short GetAsyncKeyState(int vKey);

    private readonly HookProc _proc; // держим ссылку — иначе GC уберёт делегат
    private IntPtr _hook = IntPtr.Zero;

    /// <summary>Вызывается при аварийной комбинации Ctrl+Alt+Shift+Q.</summary>
    public event Action? EmergencyExitRequested;

    public void Install()
    {
        if (_hook != IntPtr.Zero) return;
        _hook = SetWindowsHookExW(WH_KEYBOARD_LL, _proc,
            Marshal.GetHINSTANCE(typeof(KeyboardHook).Module), 0);
    }

    public KeyboardHook() => _proc = Callback;

    private static bool Down(int vk) => (GetAsyncKeyState(vk) & 0x8000) != 0;

    private IntPtr Callback(int nCode, IntPtr wParam, IntPtr lParam)
    {
        if (nCode >= 0 && (wParam == WM_KEYDOWN || wParam == WM_SYSKEYDOWN))
        {
            int vk = Marshal.ReadInt32(lParam); // первый int структуры KBDLLHOOKSTRUCT
            bool alt = Down(0x12), ctrl = Down(0x11), shift = Down(0x10);

            // аварийный выход — пропускаем и сигналим
            if (ctrl && alt && shift && vk == VK_Q)
            {
                EmergencyExitRequested?.Invoke();
                return (IntPtr)1;
            }

            bool swallow =
                vk is VK_LWIN or VK_RWIN ||        // Win-меню
                (alt && vk == VK_TAB) ||           // Alt+Tab
                (alt && vk == VK_F4) ||            // Alt+F4
                (alt && vk == VK_ESC) ||           // Alt+Esc
                (ctrl && vk == VK_ESC);            // Ctrl+Esc (Пуск)
            if (swallow) return (IntPtr)1;
        }
        return CallNextHookEx(_hook, nCode, wParam, lParam);
    }

    public void Dispose()
    {
        if (_hook != IntPtr.Zero) { UnhookWindowsHookEx(_hook); _hook = IntPtr.Zero; }
    }
}
