import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
// import 'package:flutter_gen/gen_l10n/app_localizations.dart'; // раскомментировать после flutter gen-l10n

void main() {
  runApp(const ProviderScope(child: App1448()));
}

class App1448 extends ConsumerWidget {
  const App1448({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return MaterialApp(
      title: '14:48',
      debugShowCheckedModeBanner: false,

      // ── Локализация: EN (основной) / PL / RU ──────────────────────────
      locale: const Locale('en'),
      supportedLocales: const [
        Locale('en'), // English — основной
        Locale('pl'), // Polski
        Locale('ru'), // Русский
      ],
      localizationsDelegates: const [
        // AppLocalizations.delegate,  // раскомментировать после flutter gen-l10n
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],

      // ── Тема: чёрный фон, красный акцент ─────────────────────────────
      theme: ThemeData(
        colorScheme: const ColorScheme.dark(
          primary: Color(0xFFD90025),   // 14:48 красный
          secondary: Color(0xFFA0001E),
          surface: Color(0xFF141414),
          onPrimary: Colors.white,
          onSurface: Colors.white,
        ),
        scaffoldBackgroundColor: const Color(0xFF0A0A0A),
        useMaterial3: true,
      ),

      home: const Scaffold(
        body: Center(
          child: Text(
            '14:48',
            style: TextStyle(
              fontSize: 64,
              fontWeight: FontWeight.bold,
              color: Color(0xFFD90025),
              letterSpacing: 8,
            ),
          ),
        ),
      ),
    );
  }
}
