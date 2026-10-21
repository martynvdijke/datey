package xyz.vandijke.datey

import android.webkit.CookieManager
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    private val channelName = "xyz.vandijke.datey/cookies"

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)

        // OIDC sign-in runs in a WebView, so make sure its cookie store is
        // accepting cookies before the flow starts.
        CookieManager.getInstance().setAcceptCookie(true)

        // The server's `session` cookie is HttpOnly, so the OIDC flow in the
        // WebView cannot read it from JavaScript. This channel exposes the
        // native WebView cookie store to Dart instead.
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, channelName)
            .setMethodCallHandler { call, result ->
                when (call.method) {
                    "getCookies" -> {
                        val url = call.argument<String>("url")
                        if (url.isNullOrEmpty()) {
                            result.error("invalid_url", "A url argument is required", null)
                        } else {
                            // The WebView writes cookies asynchronously; flush so
                            // the freshly issued session cookie is visible here.
                            CookieManager.getInstance().flush()
                            result.success(CookieManager.getInstance().getCookie(url))
                        }
                    }
                    "clearCookies" -> {
                        // removeAllCookies is asynchronous; clear before the
                        // OIDC flow loads so a stale session can never be
                        // captured as the new one.
                        CookieManager.getInstance().removeAllCookies {
                            CookieManager.getInstance().flush()
                            result.success(null)
                        }
                    }
                    else -> result.notImplemented()
                }
            }
    }
}
