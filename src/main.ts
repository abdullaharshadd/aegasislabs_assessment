import express, {
  Express,
  Request,
  Response,
  NextFunction,
  Router,
} from 'express';
import OpenAI from 'openai';
import { z } from 'zod';

/**
 * MIGRATION_NOTE: The source `main.py` was a Flask (NOT Django) REST API that
 * wrapped the OpenAI Completion API and managed an in-memory list of prompts.
 * It has been migrated to an idiomatic Express + TypeScript application.
 *
 * MIGRATION_NOTE: The original used the legacy `openai.Completion.create`
 * endpoint with engine `text-davinci-002`, which is deprecated and removed
 * from the modern `openai` Node SDK. The business logic (single prompt -> text
 * completion, max_tokens=150) is preserved using the Chat Completions API as
 * the closest modern equivalent. MANUAL REVIEW: confirm the model/endpoint and
 * token mapping are acceptable for your account/plan.
 *
 * MIGRATION_NOTE: The API key was hardcoded in source
 * ("YOUR_CHATGPT_API_KEY_HERE"). It is now read from the OPENAI_API_KEY
 * environment variable via a typed config accessor. Never hardcode secrets.
 *
 * MIGRATION_NOTE: The Python service kept prompts in an in-memory instance
 * attribute (`self.prompts`). This is preserved as in-memory state on the
 * service class. It is NOT persistent across restarts and NOT safe across
 * multiple worker processes. MANUAL REVIEW: migrate to Prisma/a database if
 * persistence is required.
 *
 * MIGRATION_NOTE: The API uses positional index-based identifiers
 * (/get/:index, /update/:index, /delete/:index), matching the contract used by
 * the already-migrated PromptsClient in src/client.ts. This is preserved.
 */

// ---------------------------------------------------------------------------
// Typed configuration module
// ---------------------------------------------------------------------------

interface AppConfig {
  openaiApiKey: string;
  port: number;
  openaiModel: string;
}

export function loadConfig(): AppConfig {
  const openaiApiKey = process.env.OPENAI_API_KEY;
  if (!openaiApiKey) {
    // MIGRATION_NOTE: Source hardcoded the key; we fail fast instead.
    throw new Error('OPENAI_API_KEY environment variable is required');
  }

  return {
    openaiApiKey,
    port: process.env.PORT ? Number(process.env.PORT) : 5000,
    openaiModel: process.env.OPENAI_MODEL ?? 'gpt-3.5-turbo',
  };
}

// ---------------------------------------------------------------------------
// Domain errors
// ---------------------------------------------------------------------------

/**
 * Thrown when a prompt index is out of range. Maps the source's
 * "Invalid prompt index" sentinel string to a proper error type.
 */
export class InvalidPromptIndexError extends Error {
  constructor(public readonly index: number) {
    super('Invalid prompt index');
    this.name = 'InvalidPromptIndexError';
  }
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

export interface OpenAIClientLike {
  chat: {
    completions: {
      create(args: {
        model: string;
        messages: { role: 'user'; content: string }[];
        max_tokens: number;
      }): Promise<{ choices: { message: { content: string | null } }[] }>;
    };
  };
}

/**
 * Port of the Python `ChatGPTBotAPI` class.
 *
 * Uses constructor dependency injection (OpenAI client + model name) so the
 * service can be unit tested with a mock client.
 */
export class ChatGPTBotService {
  private readonly prompts: string[] = [];

  constructor(
    private readonly openai: OpenAIClientLike,
    private readonly model: string,
  ) {}

  createPrompt(prompt: string): void {
    this.prompts.push(prompt);
  }

  private assertValidIndex(index: number): void {
    if (
      !Number.isInteger(index) ||
      index < 0 ||
      index >= this.prompts.length
    ) {
      throw new InvalidPromptIndexError(index);
    }
  }

  async getResponse(promptIndex: number): Promise<string> {
    this.assertValidIndex(promptIndex);

    const prompt = this.prompts[promptIndex];

    // MIGRATION_NOTE: openai.Completion.create(engine="text-davinci-002", ...)
    // replaced with Chat Completions. max_tokens=150 preserved.
    const response = await this.openai.chat.completions.create({
      model: this.model,
      messages: [{ role: 'user', content: prompt }],
      max_tokens: 150,
    });

    return response.choices[0]?.message.content ?? '';
  }

  updatePrompt(promptIndex: number, newPrompt: string): string {
    this.assertValidIndex(promptIndex);
    this.prompts[promptIndex] = newPrompt;
    return 'Prompt updated successfully';
  }

  deletePrompt(promptIndex: number): string {
    this.assertValidIndex(promptIndex);
    this.prompts.splice(promptIndex, 1);
    return 'Prompt deleted successfully';
  }
}

// ---------------------------------------------------------------------------
// Validation schemas
// ---------------------------------------------------------------------------

const createPromptSchema = z.object({
  // Source rejected empty/missing prompts with 400 "Prompt not provided".
  prompt: z.string().min(1, 'Prompt not provided'),
});

const updatePromptSchema = z.object({
  new_prompt: z.string().min(1, 'New prompt not provided'),
});

const indexParamSchema = z.coerce
  .number()
  .int('Invalid prompt index')
  .nonnegative('Invalid prompt index');

// ---------------------------------------------------------------------------
// Router
// ---------------------------------------------------------------------------

export function createPromptsRouter(service: ChatGPTBotService): Router {
  const router = Router();

  router.post('/create', (req: Request, res: Response) => {
    const parsed = createPromptSchema.safeParse(req.body);
    if (!parsed.success) {
      // Preserve original 400 message shape.
      return res.status(400).json({ error: 'Prompt not provided' });
    }

    service.createPrompt(parsed.data.prompt);
    return res.status(201).json({ message: 'Prompt created successfully' });
  });

  router.get(
    '/get/:promptIndex',
    async (req: Request, res: Response, next: NextFunction) => {
      const parsedIndex = indexParamSchema.safeParse(req.params.promptIndex);
      if (!parsedIndex.success) {
        return res.status(200).json({ response: 'Invalid prompt index' });
      }

      try {
        const response = await service.getResponse(parsedIndex.data);
        return res.status(200).json({ response });
      } catch (err) {
        if (err instanceof InvalidPromptIndexError) {
          // MIGRATION_NOTE: Source returned the sentinel string with HTTP 200.
          return res.status(200).json({ response: 'Invalid prompt index' });
        }
        return next(err);
      }
    },
  );

  router.delete('/delete/:promptIndex', (req: Request, res: Response) => {
    const parsedIndex = indexParamSchema.safeParse(req.params.promptIndex);
    if (!parsedIndex.success) {
      return res.status(200).json({ message: 'Invalid prompt index' });
    }

    try {
      const message = service.deletePrompt(parsedIndex.data);
      return res.status(200).json({ message });
    } catch (err) {
      if (err instanceof InvalidPromptIndexError) {
        return res.status(200).json({ message: 'Invalid prompt index' });
      }
      throw err;
    }
  });

  router.put('/update/:promptIndex', (req: Request, res: Response) => {
    const parsedBody = updatePromptSchema.safeParse(req.body);
    if (!parsedBody.success) {
      return res.status(400).json({ error: 'New prompt not provided' });
    }

    const parsedIndex = indexParamSchema.safeParse(req.params.promptIndex);
    if (!parsedIndex.success) {
      return res.status(200).json({ message: 'Invalid prompt index' });
    }

    try {
      const message = service.updatePrompt(
        parsedIndex.data,
        parsedBody.data.new_prompt,
      );
      return res.status(200).json({ message });
    } catch (err) {
      if (err instanceof InvalidPromptIndexError) {
        return res.status(200).json({ message: 'Invalid prompt index' });
      }
      throw err;
    }
  });

  return router;
}

// ---------------------------------------------------------------------------
// App factory + centralized error handling
// ---------------------------------------------------------------------------

export function createApp(service: ChatGPTBotService): Express {
  const app = express();
  app.use(express.json());
  app.use('/', createPromptsRouter(service));

  // Centralized error middleware. Consistent JSON error responses.
  app.use(
    (err: unknown, _req: Request, res: Response, _next: NextFunction) => {
      const message =
        err instanceof Error ? err.message : 'Internal server error';
      res.status(500).json({ error: message });
    },
  );

  return app;
}

// ---------------------------------------------------------------------------
// Bootstrap
// ---------------------------------------------------------------------------

export function bootstrap(): void {
  const config = loadConfig();
  const openai = new OpenAI({ apiKey: config.openaiApiKey });

  // MIGRATION_NOTE: The official `openai` SDK's `chat.completions.create`
  // matches the OpenAIClientLike interface structurally.
  const service = new ChatGPTBotService(
    openai as unknown as OpenAIClientLike,
    config.openaiModel,
  );
  const app = createApp(service);

  app.listen(config.port, () => {
    // MIGRATION_NOTE: Flask's debug=True has no direct equivalent; use
    // NODE_ENV / a logger config for development behavior.
    // eslint-disable-next-line no-console
    console.log(`Server listening on port ${config.port}`);
  });
}

// Run when executed directly (mirrors `if __name__ == '__main__'`).
if (require.main === module) {
  bootstrap();
}
