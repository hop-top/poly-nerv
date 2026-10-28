<?php

declare(strict_types=1);

namespace HopTop\Nerv\Tests;

use HopTop\Nerv\Client;
use PHPUnit\Framework\Attributes\CoversClass;
use PHPUnit\Framework\TestCase;

/**
 * Covers {@see Client::defaultSocketPath()} + {@see Client::ENV_SOCKET}.
 *
 * Mirrors the Go SDK's TestDefaultSocketPath_* cases. Env vars are
 * mutated and restored per-test via the setUp/tearDown pair so tests
 * remain isolated.
 */
#[CoversClass(Client::class)]
final class DefaultSocketPathTest extends TestCase
{
    /** @var array<string, string|false> */
    private array $priorEnv = [];

    protected function setUp(): void
    {
        foreach (['NERV_SOCKET', 'XDG_RUNTIME_DIR'] as $key) {
            $this->priorEnv[$key] = getenv($key);
        }
    }

    protected function tearDown(): void
    {
        foreach ($this->priorEnv as $key => $value) {
            if ($value === false) {
                putenv($key);
            } else {
                putenv($key . '=' . $value);
            }
        }
    }

    public function testEnvSocketConstantMatchesDaemonConvention(): void
    {
        self::assertSame('NERV_SOCKET', Client::ENV_SOCKET);
    }

    public function testDefaultSocketPathRespectsEnv(): void
    {
        putenv(Client::ENV_SOCKET . '=/custom/path.sock');
        putenv('XDG_RUNTIME_DIR=/run/user/1000');

        self::assertSame('/custom/path.sock', Client::defaultSocketPath());
    }

    public function testDefaultSocketPathXdgFallback(): void
    {
        putenv(Client::ENV_SOCKET);
        putenv('XDG_RUNTIME_DIR=/run/user/1000');

        self::assertSame(
            '/run/user/1000' . DIRECTORY_SEPARATOR . 'nerv.sock',
            Client::defaultSocketPath(),
        );
    }

    public function testDefaultSocketPathTmpFallbackWhenXdgUnset(): void
    {
        putenv(Client::ENV_SOCKET);
        putenv('XDG_RUNTIME_DIR');

        $expected = rtrim(sys_get_temp_dir(), DIRECTORY_SEPARATOR)
            . DIRECTORY_SEPARATOR
            . 'nerv.sock';
        self::assertSame($expected, Client::defaultSocketPath());
    }

    public function testDefaultSocketPathTreatsEmptyEnvSocketAsUnset(): void
    {
        putenv(Client::ENV_SOCKET . '=');
        putenv('XDG_RUNTIME_DIR=/run/user/1000');

        self::assertSame(
            '/run/user/1000' . DIRECTORY_SEPARATOR . 'nerv.sock',
            Client::defaultSocketPath(),
        );
    }
}
