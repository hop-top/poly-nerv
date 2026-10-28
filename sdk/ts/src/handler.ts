/**
 * nerv envelope — wire format delivered on stdin.
 * Matches spec/envelope.schema.json.
 */
export interface Envelope {
  nerv_version: string;
  event: string;
  event_id: string;
  timestamp: string;
  cli: { name: string; version: string; binary_path?: string };
  session: { id: string; project_key?: string; cwd?: string };
  payload: Record<string, unknown>;
  synthesized?: boolean;
  synthesis_source?: string | null;
  capabilities?: { cli: string; cli_version_range: string } | null;
}

/**
 * nerv decision — written to stdout.
 * Action MUST match exit code (allow=0, warn=1, block=2, rewrite=3).
 */
export interface Decision {
  action: 'allow' | 'warn' | 'block' | 'rewrite';
  message?: string;
  rewrite?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

/** Handler function: receives envelope, returns decision. */
export type Handler = (envelope: Envelope) => Decision | Promise<Decision>;

const EXIT_CODES: Record<Decision['action'], number> = {
  allow: 0,
  warn: 1,
  block: 2,
  rewrite: 3,
};

/**
 * Create a nerv hook handler.
 *
 * Reads a JSON envelope from stdin, calls `fn`, writes the
 * decision to stdout, and exits with the matching exit code.
 */
export function createHandler(fn: Handler): void {
  const chunks: Buffer[] = [];

  process.stdin.on('data', (chunk) => chunks.push(chunk));
  process.stdin.on('end', async () => {
    try {
      const raw = Buffer.concat(chunks).toString('utf-8');
      const envelope: Envelope = JSON.parse(raw);
      const decision = await fn(envelope);

      const exitCode = EXIT_CODES[decision.action as Decision['action']];
      if (exitCode === undefined) {
        process.stderr.write(
          `nerv-sdk: invalid action: ${JSON.stringify(decision.action)}\n`
        );
        process.exit(4);
        return;
      }

      process.stdout.write(JSON.stringify(decision) + '\n', () => {
        process.exit(exitCode);
      });
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      process.stderr.write(`nerv-sdk: handler error: ${msg}\n`);
      // exit >= 4 = error -> fail-open
      process.exit(4);
    }
  });
}
