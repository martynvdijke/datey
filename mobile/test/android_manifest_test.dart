import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  final manifest =
      File('android/app/src/main/AndroidManifest.xml').readAsStringSync();
  final gradle = File('android/app/build.gradle.kts').readAsStringSync();

  test('manifest grants INTERNET', () {
    expect(manifest, contains('android.permission.INTERNET'));
  });

  test('manifest permits self-hosted cleartext HTTP', () {
    expect(manifest, contains('android:usesCleartextTraffic="true"'));
  });

  test('manifest declares a round launcher icon', () {
    expect(manifest, contains('android:roundIcon="@mipmap/ic_launcher_round"'));
  });

  test('adaptive launcher icon uses vector background, foreground and monochrome layers', () {
    final adaptive = File(
      'android/app/src/main/res/mipmap-anydpi-v26/ic_launcher.xml',
    ).readAsStringSync();
    expect(adaptive, contains('@drawable/ic_launcher_background'));
    expect(adaptive, contains('@drawable/ic_launcher_foreground'));
    expect(adaptive, contains('@drawable/ic_launcher_monochrome'));

    for (final layer in [
      'ic_launcher_background',
      'ic_launcher_foreground',
      'ic_launcher_monochrome',
    ]) {
      expect(
        File('android/app/src/main/res/drawable/$layer.xml').existsSync(),
        isTrue,
      );
    }

    for (final density in ['mdpi', 'hdpi', 'xhdpi', 'xxhdpi', 'xxxhdpi']) {
      final dir = 'android/app/src/main/res/mipmap-$density';
      expect(File('$dir/ic_launcher.png').existsSync(), isTrue);
      expect(File('$dir/ic_launcher_round.png').existsSync(), isTrue);
    }
  });

  test('main activity exposes the WebView cookie bridge used by OIDC', () {
    final activity = File(
      'android/app/src/main/kotlin/xyz/vandijke/datey/MainActivity.kt',
    ).readAsStringSync();
    expect(activity, contains('xyz.vandijke.datey/cookies'));
    expect(activity, contains('"getCookies"'));
    expect(activity, contains('"clearCookies"'));
  });

  test('release build wires key.properties signing with a debug fallback', () {
    expect(gradle, contains('key.properties'));
    expect(gradle, contains('releaseSigningConfig'));
    expect(gradle, contains('signingConfigs.getByName("debug")'));
  });
}
