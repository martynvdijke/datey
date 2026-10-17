import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'providers/auth_provider.dart';
import 'screens/onboarding_screen.dart';
import 'screens/dashboard_screen.dart';
import 'screens/people_screen.dart';
import 'screens/person_detail_screen.dart';
import 'screens/settings_screen.dart';

final routerProvider = Provider<GoRouter>((ref) {
  final auth = ref.watch(authStateProvider);
  return GoRouter(
    initialLocation: '/',
    redirect: (context, state) {
      final isAuth = auth.whenOrNull(data: (u) => u != null) ?? false;
      final isLoading = auth.isLoading;
      final onOnboarding = state.matchedLocation == '/onboarding';
      if (isLoading) return null;
      if (!isAuth && !onOnboarding) return '/onboarding';
      if (isAuth && onOnboarding) return '/';
      return null;
    },
    routes: [
      GoRoute(path: '/onboarding', builder: (ctx, s) => const OnboardingScreen()),
      ShellRoute(
        builder: (ctx, state, child) => AppShell(child: child),
        routes: [
          GoRoute(path: '/', builder: (ctx, s) => const DashboardScreen()),
          GoRoute(path: '/people', builder: (ctx, s) => const PeopleScreen()),
          GoRoute(path: '/people/:id', builder: (ctx, s) {
            final id = int.tryParse(s.pathParameters['id'] ?? '') ?? 0;
            return PersonDetailScreen(personId: id);
          }),
          GoRoute(path: '/settings', builder: (ctx, s) => const SettingsScreen()),
        ],
      ),
    ],
  );
});

class AppShell extends StatelessWidget {
  final Widget child;
  const AppShell({super.key, required this.child});

  int _index(String loc) {
    if (loc.startsWith('/people')) return 1;
    if (loc.startsWith('/settings')) return 2;
    return 0;
  }

  @override
  Widget build(BuildContext context) {
    final loc = GoRouterState.of(context).matchedLocation;
    final idx = _index(loc);
    return Scaffold(
      body: child,
      bottomNavigationBar: NavigationBar(
        selectedIndex: idx,
        onDestinationSelected: (i) {
          switch (i) {
            case 0:
              context.go('/');
              break;
            case 1:
              context.go('/people');
              break;
            case 2:
              context.go('/settings');
              break;
          }
        },
        destinations: const [
          NavigationDestination(icon: Icon(Icons.celebration_outlined), selectedIcon: Icon(Icons.celebration), label: 'Upcoming'),
          NavigationDestination(icon: Icon(Icons.people_outline), selectedIcon: Icon(Icons.people), label: 'People'),
          NavigationDestination(icon: Icon(Icons.settings_outlined), selectedIcon: Icon(Icons.settings), label: 'Settings'),
        ],
      ),
    );
  }
}
