<?php

declare(strict_types=1);

namespace HopTop\Nerv\Tests;

use HopTop\Nerv\Client;
use HopTop\Nerv\EventServiceStub;
use Nerv\V1\Action;
use Nerv\V1\Decision;
use Nerv\V1\Envelope;
use Nerv\V1\EventDescriptor;
use PHPUnit\Framework\Attributes\CoversClass;
use PHPUnit\Framework\TestCase;

#[CoversClass(Client::class)]
#[CoversClass(EventServiceStub::class)]
final class ClientTest extends TestCase
{
    public function testDispatchReturnsQueuedDecisionFromStubTransport(): void
    {
        $queued = new Decision();
        $queued->setAction(Action::ACTION_BLOCK);
        $queued->setMessage('blocked by stub');

        $stub = (new EventServiceStub())->withQueuedDecision($queued);
        $client = new Client($stub);

        $envelope = new Envelope();
        $envelope->setEvent('PreToolUse');

        $decision = $client->dispatch($envelope);

        self::assertSame(Action::ACTION_BLOCK, $decision->getAction());
        self::assertSame('blocked by stub', $decision->getMessage());
        self::assertCount(1, $stub->dispatchedEnvelopes());
        self::assertSame('PreToolUse', $stub->dispatchedEnvelopes()[0]->getEvent());
    }

    public function testListSurfacesDescriptorsFromTransport(): void
    {
        $descriptor = new EventDescriptor();
        $descriptor->setName('PreToolUse');

        $stub = (new EventServiceStub())->withDescriptors([$descriptor]);
        $client = new Client($stub);

        $descriptors = $client->list();

        self::assertCount(1, $descriptors);
        self::assertSame('PreToolUse', $descriptors[0]->getName());
    }

    public function testCloseIsIdempotent(): void
    {
        $client = new Client(new EventServiceStub());
        $client->close();
        $client->close();
        self::assertTrue(true); // no exception
    }

    public function testFromSocketRequiresExtGrpcWhenAbsent(): void
    {
        if (extension_loaded('grpc')) {
            // With ext-grpc loaded, fromSocket succeeds and returns a
            // Client wired to a GrpcTransport. No connection attempt
            // happens at construction time — the channel is lazy.
            $client = Client::fromSocket('/tmp/nerv-unit-test.sock');
            self::assertInstanceOf(Client::class, $client);
            $client->close();
            return;
        }

        // Without ext-grpc, GrpcTransport's constructor throws with
        // install instructions, even though grpc/grpc's BaseStub
        // (pure PHP) is installed.
        self::assertTrue(class_exists(\Grpc\BaseStub::class), 'grpc/grpc not installed');
        $this->expectException(\RuntimeException::class);
        $this->expectExceptionMessageMatches('/ext-grpc|PECL grpc/');
        Client::fromSocket('/tmp/nerv-unit-test.sock');
    }

    public function testFromBaseUrlAcceptsTlsSchemeWhenExtensionLoaded(): void
    {
        if (!extension_loaded('grpc')) {
            // Without ext-grpc, GrpcTransport's constructor surfaces
            // the canonical install-instructions error rather than a
            // class-not-found from ChannelCredentials.
            $this->expectException(\RuntimeException::class);
            $this->expectExceptionMessageMatches('/ext-grpc|PECL grpc/');
            Client::fromBaseUrl('grpcs://example.invalid:443');
            return;
        }
        $client = Client::fromBaseUrl('grpcs://example.invalid:443');
        self::assertInstanceOf(Client::class, $client);
        $client->close();
    }

    public function testFromBaseUrlRejectsEmptyUrl(): void
    {
        $this->expectException(\InvalidArgumentException::class);
        Client::fromBaseUrl('');
    }

    public function testFromBaseUrlRejectsUnsupportedScheme(): void
    {
        $this->expectException(\InvalidArgumentException::class);
        $this->expectExceptionMessageMatches('/unsupported URL scheme/');
        Client::fromBaseUrl('redis://example.invalid:6379');
    }
}
