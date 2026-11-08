import 'dart:convert';
import 'dart:typed_data';

import 'package:datey/src/telemetry/telemetry.dart';
import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';

/// Fake Dio transport: no network is touched.
class _FakeAdapter implements HttpClientAdapter {
  _FakeAdapter(this.responder);

  final Future<ResponseBody> Function(RequestOptions options) responder;

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) =>
      responder(options);

  @override
  void close({bool force = false}) {}
}

ResponseBody _jsonBody(Object body, int statusCode) => ResponseBody.fromString(
      jsonEncode(body),
      statusCode,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType],
      },
    );

Dio _dioWith(HttpClientAdapter adapter) {
  final dio = Dio(BaseOptions(
    connectTimeout: const Duration(seconds: 1),
    receiveTimeout: const Duration(seconds: 1),
    validateStatus: (status) => status != null && status < 400,
  ));
  dio.httpClientAdapter = adapter;
  return dio;
}

void main() {
  group('normalizeEndpoint', () {
    test('appends /v1/traces and trims trailing slashes', () {
      expect(
        Telemetry.normalizeEndpoint('https://collector.example.com:4318/'),
        'https://collector.example.com:4318/v1/traces',
      );
    });

    test('leaves an existing /v1/traces suffix intact', () {
      expect(
        Telemetry.normalizeEndpoint(
          'https://collector.example.com:4318/v1/traces',
        ),
        'https://collector.example.com:4318/v1/traces',
      );
    });

    test('trims surrounding whitespace', () {
      expect(
        Telemetry.normalizeEndpoint('  https://otel.example.com  '),
        'https://otel.example.com/v1/traces',
      );
    });
  });

  group('parseOtelHeaders', () {
    test('parses comma-separated key=value pairs', () {
      expect(
        Telemetry.parseOtelHeaders(
          'Authorization=Bearer%20token,X-Scope=datey',
        ),
        {'Authorization': 'Bearer token', 'X-Scope': 'datey'},
      );
    });

    test('skips malformed pairs and empty values', () {
      expect(
        Telemetry.parseOtelHeaders('novalue,=empty,key=,good=1'),
        {'good': '1'},
      );
    });

    test('returns an empty map for empty input', () {
      expect(Telemetry.parseOtelHeaders(''), isEmpty);
    });
  });

  group('buildOtlpPayload', () {
    test('builds resource, scope and span with attribute typing', () {
      final span = TraceSpan(
        name: 'screen dashboard',
        attributes: const {'screen.name': 'dashboard'},
        traceId: 'a' * 32,
        spanId: 'b' * 16,
        startNanos: 1000,
        kind: 2,
      );

      final payload = buildOtlpPayload(
        serviceName: 'datey-android',
        serviceVersion: '1.2.3',
        span: span,
        attributes: const {
          'count': 3,
          'ratio': 1.5,
          'ok': true,
          'label': 'value',
        },
        failed: true,
        endNanos: 2000,
      );

      final resourceSpans = payload['resourceSpans'] as List;
      final resource = (resourceSpans.first as Map)['resource'] as Map;
      final resourceAttrs = resource['attributes'] as List;
      final resourceMap = {
        for (final attr in resourceAttrs)
          (attr as Map)['key']: (attr['value'] as Map).values.first,
      };
      expect(resourceMap['service.name'], 'datey-android');
      expect(resourceMap['service.version'], '1.2.3');
      expect(resourceMap['telemetry.sdk.name'], 'datey-mobile');
      expect(resourceMap['telemetry.sdk.language'], 'dart');

      final scope = ((resourceSpans.first as Map)['scopeSpans'] as List).first
          as Map;
      expect((scope['scope'] as Map)['name'], 'datey.mobile');

      final outSpan = (scope['spans'] as List).first as Map;
      expect(outSpan['traceId'], 'a' * 32);
      expect(outSpan['spanId'], 'b' * 16);
      expect(outSpan['name'], 'screen dashboard');
      expect(outSpan['kind'], 2);
      expect(outSpan['startTimeUnixNano'], '1000');
      expect(outSpan['endTimeUnixNano'], '2000');
      expect((outSpan['status'] as Map)['code'], 2);

      final spanAttrs = {
        for (final attr in outSpan['attributes'] as List)
          (attr as Map)['key']: (attr['value'] as Map),
      };
      expect(spanAttrs['count'], {'intValue': '3'});
      expect(spanAttrs['ratio'], {'doubleValue': 1.5});
      expect(spanAttrs['ok'], {'boolValue': true});
      expect(spanAttrs['label'], {'stringValue': 'value'});
    });

    test('sets status code 1 when the span succeeded', () {
      final span = TraceSpan(
        name: 'ok',
        attributes: const {},
        traceId: 'a' * 32,
        spanId: 'b' * 16,
        startNanos: 1,
      );
      final payload = buildOtlpPayload(
        serviceName: 'datey-android',
        serviceVersion: '1.0.0',
        span: span,
        attributes: const {},
        failed: false,
        endNanos: 2,
      );
      final scope = ((payload['resourceSpans'] as List).first as Map)[
          'scopeSpans'] as List;
      final outSpan = ((scope.first as Map)['spans'] as List).first as Map;
      expect((outSpan['status'] as Map)['code'], 1);
    });
  });

  group('OtelSettings.fromJson', () {
    test('parses the canonical API shape', () {
      final settings = OtelSettings.fromJson(const {
        'endpoint': 'https://collector.example.com:4318',
        'enabled': true,
        'traces_enabled': true,
        'metrics_enabled': false,
        'logs_enabled': false,
      });
      expect(settings.endpoint, 'https://collector.example.com:4318');
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
      expect(settings.metricsEnabled, isFalse);
      expect(settings.logsEnabled, isFalse);
      expect(settings.isUsable, isTrue);
    });

    test('accepts aliases and string/number booleans', () {
      final settings = OtelSettings.fromJson(const {
        'otel_endpoint': 'https://otel.example.com',
        'otel_enabled': 'true',
        'traces_enabled': 1,
      });
      expect(settings.endpoint, 'https://otel.example.com');
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
    });

    test('defaults traces to enabled', () {
      final settings = OtelSettings.fromJson(const {
        'endpoint': 'https://otel.example.com',
        'enabled': true,
      });
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
    });

    test('disables everything when the endpoint is empty', () {
      final settings = OtelSettings.fromJson(const {
        'endpoint': '',
        'enabled': true,
        'traces_enabled': true,
        'metrics_enabled': true,
        'logs_enabled': true,
      });
      expect(settings.enabled, isFalse);
      expect(settings.tracesEnabled, isFalse);
      expect(settings.metricsEnabled, isFalse);
      expect(settings.logsEnabled, isFalse);
      expect(settings.isUsable, isFalse);
    });

    test('tolerates missing keys', () {
      final settings = OtelSettings.fromJson(const {});
      expect(settings.endpoint, '');
      expect(settings.enabled, isFalse);
      expect(settings.tracesEnabled, isFalse);
    });
  });

  group('fetchOtelSettings', () {
    test('returns parsed settings on 200', () async {
      final dio = _dioWith(_FakeAdapter((options) async {
        expect(options.uri.toString(), 'https://datey.example.com/api/otel-settings');
        return _jsonBody({
          'endpoint': 'https://collector.example.com:4318',
          'enabled': true,
          'traces_enabled': true,
          'metrics_enabled': false,
          'logs_enabled': false,
        }, 200);
      }));

      final settings = await fetchOtelSettings(
        'https://datey.example.com/',
        dio: dio,
      );

      expect(settings, isNotNull);
      expect(settings!.endpoint, 'https://collector.example.com:4318');
      expect(settings.enabled, isTrue);
      expect(settings.tracesEnabled, isTrue);
    });

    test('returns null on a non-200 response', () async {
      final dio = _dioWith(
        _FakeAdapter((_) async => _jsonBody({'error': 'nope'}, 404)),
      );

      final settings = await fetchOtelSettings('https://datey.example.com', dio: dio);
      expect(settings, isNull);
    });

    test('returns null instead of throwing on transport errors', () async {
      final dio = _dioWith(_FakeAdapter((_) async {
        throw DioException(
          requestOptions: RequestOptions(path: '/api/otel-settings'),
          message: 'offline',
        );
      }));

      final settings = await fetchOtelSettings('https://datey.example.com', dio: dio);
      expect(settings, isNull);
    });

    test('returns null for an empty base URL', () async {
      final settings = await fetchOtelSettings('');
      expect(settings, isNull);
    });
  });
}
