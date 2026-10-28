<?php

declare(strict_types=1);

namespace HopTop\Nerv\Tests;

use HopTop\Nerv\Client;
use HopTop\Nerv\GrpcTransport;
use Nerv\V1\CLIInfo;
use Nerv\V1\Envelope;
use Nerv\V1\SessionInfo;
use PHPUnit\Framework\Attributes\CoversClass;
use PHPUnit\Framework\TestCase;

/**
 * Real-wire integration tests for {@see GrpcTransport}.
 *
 * Two coverage tiers:
 *
 *   - Static checks (always run): GrpcTransport constructor throws a
 *     clear error when the PECL `grpc` extension is absent;
 *     fromSocket/fromBaseUrl factories succeed when it is present
 *     (channels are lazy — no connection at construction time).
 *
 *   - Round-trip checks (require both `ext-grpc` AND a live daemon
 *     socket via `NERV_TEST_SOCKET` env var): dispatch + list against
 *     the running daemon. The parity fake service
 *     (test/parity/parity_test.go) is the canonical fixture; rebooked
 *     for unit-suite use by exporting NERV_TEST_SOCKET when running
 *     phpunit inside the parity harness. In CI, the dedicated
 *     `TestParity_PHP` covers the full round-trip — these unit-tier
 *     round-trip checks skip cleanly when no socket is supplied.
 */
#[CoversClass(GrpcTransport::class)]
#[CoversClass(Client::class)]
final class WireTest extends TestCase
{
    public function testGrpcTransportConstructorThrowsWhenExtensionAbsent(): void
    {
        if (extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc loaded; absence-path covered in CI matrix entries without the extension');
        }
        // grpc/grpc's BaseStub is installed regardless of the
        // extension; the guard must key on the extension itself.
        self::assertTrue(class_exists(\Grpc\BaseStub::class), 'grpc/grpc not installed');
        $this->expectException(\RuntimeException::class);
        $this->expectExceptionMessageMatches('/ext-grpc|PECL grpc/');
        new GrpcTransport('unix:/tmp/nerv-wiretest.sock');
    }

    public function testFromSocketConstructsClientWhenExtensionLoaded(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromSocket('/tmp/nerv-wiretest-unused.sock');
        self::assertInstanceOf(Client::class, $client);
        $client->close();
    }

    public function testFromBaseUrlConstructsClientWhenExtensionLoaded(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromBaseUrl('grpc://127.0.0.1:50051');
        self::assertInstanceOf(Client::class, $client);
        $client->close();
    }

    /**
     * Factory smoke for the TLS schemes: grpcs:// + https:// both
     * yield a Client wrapping a GrpcTransport. The PECL channel is
     * lazy — no network handshake at construction — so this asserts
     * the wiring picks ChannelCredentials::createSsl() without
     * blowing up on a class-not-found.
     *
     * Real TLS round-trip is out of scope (would need a TLS-bound
     * test server fixture); coverage stops at the factory boundary.
     */
    public function testFromBaseUrlWiresTlsCredentialsForGrpcsScheme(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromBaseUrl('grpcs://example.com:443');
        self::assertInstanceOf(Client::class, $client);
        self::assertInstanceOf(GrpcTransport::class, $client->transport());
        $client->close();
    }

    public function testFromBaseUrlWiresTlsCredentialsForHttpsScheme(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromBaseUrl('https://example.com:443');
        self::assertInstanceOf(Client::class, $client);
        self::assertInstanceOf(GrpcTransport::class, $client->transport());
        $client->close();
    }

    /**
     * Insecure regression: grpc:// + http:// + bare host:port keep
     * the insecure credentials path. Mirrors the TLS smoke shape so
     * future scheme changes don't silently flip insecure → TLS.
     */
    public function testFromBaseUrlKeepsInsecureCredentialsForHttpScheme(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromBaseUrl('http://127.0.0.1:50051');
        self::assertInstanceOf(Client::class, $client);
        self::assertInstanceOf(GrpcTransport::class, $client->transport());
        $client->close();
    }

    public function testFromBaseUrlKeepsInsecureCredentialsForBareHostPort(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $client = Client::fromBaseUrl('127.0.0.1:50051');
        self::assertInstanceOf(Client::class, $client);
        self::assertInstanceOf(GrpcTransport::class, $client->transport());
        $client->close();
    }

    public function testRoundTripAgainstLiveDaemon(): void
    {
        if (!extension_loaded('grpc')) {
            self::markTestSkipped('ext-grpc not loaded; install via `pecl install grpc`');
        }
        $socket = getenv('NERV_TEST_SOCKET');
        if (!is_string($socket) || $socket === '') {
            self::markTestSkipped(
                'NERV_TEST_SOCKET not set; full round-trip covered by '
                . 'test/parity/parity_test.go TestParity_PHP in CI'
            );
        }
        if (!file_exists($socket)) {
            self::fail('NERV_TEST_SOCKET points at a missing path: ' . $socket);
        }

        $client = Client::fromSocket($socket);
        try {
            // List: parity fixture returns one descriptor named "PreToolUse".
            $events = $client->list();
            self::assertNotEmpty($events, 'List returned no event descriptors');

            // Dispatch: parity fixture's canonical envelope.
            $envelope = new Envelope();
            $envelope->setNervVersion('1.0.0-draft');
            $envelope->setEvent('PreToolUse');
            $envelope->setEventId('wiretest-event');
            $cli = new CLIInfo();
            $cli->setName('claude');
            $cli->setVersion('0.0.0');
            $envelope->setCli($cli);
            $session = new SessionInfo();
            $session->setId('wiretest-session');
            $session->setCwd('/tmp');
            $envelope->setSession($session);

            $decision = $client->dispatch($envelope);
            self::assertNotNull($decision);
            self::assertNotSame('', $decision->getMessage(), 'decision missing message');
        } finally {
            $client->close();
        }
    }
}
