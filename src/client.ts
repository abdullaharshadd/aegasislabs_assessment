import axios, { AxiosInstance, AxiosError } from 'axios';

/**
 * HTTP client for the prompts CRUD REST API.
 *
 * MIGRATION_NOTE: The original Python script was a standalone `requests`-based
 * client/test harness, NOT Django application code. It has been migrated to an
 * idiomatic TypeScript class with dependency injection (the axios instance can
 * be supplied for testing/mocking).
 *
 * MIGRATION_NOTE: BASE_URL was hardcoded to http://127.0.0.1:5000 in the source.
 * It is now externalized to the PROMPTS_API_BASE_URL environment variable with
 * the original value retained as a fallback default.
 *
 * MIGRATION_NOTE: The API uses positional index-based identifiers
 * (/get/{index}, /update/{index}, /delete/{index}) rather than stable IDs.
 * This contract is preserved as-is; coordinate with the API owner if IDs are
 * preferred.
 */

export interface ApiErrorResponse {
  error: string;
  details?: unknown;
}

export type PromptApiResponse = unknown;

const DEFAULT_BASE_URL = 'http://127.0.0.1:5000';

/**
 * Typed config accessor. In a larger service this would live in a shared
 * config module; kept inline here since this is a single-file client.
 */
export function getBaseUrl(): string {
  return process.env.PROMPTS_API_BASE_URL ?? DEFAULT_BASE_URL;
}

export class PromptsClient {
  private readonly http: AxiosInstance;

  constructor(baseUrl: string = getBaseUrl(), http?: AxiosInstance) {
    this.http =
      http ??
      axios.create({
        baseURL: baseUrl,
        headers: { 'Content-Type': 'application/json' },
        // Do not throw on non-2xx; mirror the Python script which only
        // guarded against invalid JSON, not HTTP status codes.
        validateStatus: () => true,
      });
  }

  /**
   * Create a new prompt. Mirrors POST /create with body { prompt }.
   */
  async createPrompt(prompt: string): Promise<PromptApiResponse> {
    const response = await this.http.post('/create', { prompt });
    return this.parseBody(response.data);
  }

  /**
   * Fetch a prompt response by positional index. Mirrors GET /get/{index}.
   *
   * MIGRATION_NOTE: The Python original caught JSONDecodeError here. axios
   * parses JSON automatically, so we instead guard against a missing/invalid
   * body and surface the equivalent error shape.
   */
  async getResponse(promptIndex: number): Promise<PromptApiResponse> {
    const response = await this.http.get(`/get/${promptIndex}`);
    return this.parseBody(response.data);
  }

  /**
   * Update a prompt by positional index. Mirrors PUT /update/{index}
   * with body { new_prompt }.
   */
  async updatePrompt(
    promptIndex: number,
    newPrompt: string,
  ): Promise<PromptApiResponse> {
    const response = await this.http.put(`/update/${promptIndex}`, {
      new_prompt: newPrompt,
    });
    return this.parseBody(response.data);
  }

  /**
   * Delete a prompt by positional index. Mirrors DELETE /delete/{index}.
   */
  async deletePrompt(promptIndex: number): Promise<PromptApiResponse> {
    const response = await this.http.delete(`/delete/${promptIndex}`);
    return this.parseBody(response.data);
  }

  /**
   * Equivalent of the Python JSONDecodeError guard. axios already deserializes
   * JSON; if the server returned a non-JSON payload (e.g. an HTML error page),
   * the body arrives as a string, which we treat as an invalid response.
   */
  private parseBody(data: unknown): PromptApiResponse {
    if (data === null || data === undefined || typeof data === 'string') {
      const err: ApiErrorResponse = {
        error: 'Invalid response from the server',
      };
      return err;
    }
    return data;
  }
}

/**
 * Sequential exercise of the CRUD endpoints. This preserves the behaviour of
 * the original `main()` test script.
 *
 * MIGRATION_NOTE: The original main() was an ad-hoc test harness, not
 * application logic. For a production migration this should become Jest +
 * supertest integration tests against the API. It is retained here so the
 * file can still be run directly: `ts-node src/client.ts`.
 */
export async function main(client: PromptsClient = new PromptsClient()): Promise<void> {
  // Test createPrompt
  const prompt1 = 'What is life?';
  const prompt2 = 'What is the capital of Pakistan?';
  const createResponse1 = await client.createPrompt(prompt1);
  const createResponse2 = await client.createPrompt(prompt2);

  console.log(createResponse1);
  console.log(createResponse2);

  // Test getResponse
  const getResponseResult = await client.getResponse(0);
  console.log(getResponseResult);

  // Test updatePrompt
  const updateIndex = 1;
  const newPrompt = 'Who is Goku?';
  const updateResponse = await client.updatePrompt(updateIndex, newPrompt);
  console.log(updateResponse);

  // Test getResponse after update
  const responseAfterUpdate = await client.getResponse(updateIndex);
  console.log(responseAfterUpdate);

  // Test deletePrompt
  const deleteIndex = 0;
  const deleteResponse = await client.deletePrompt(deleteIndex);
  console.log(deleteResponse);

  // Test getResponse after delete
  const responseAfterDelete = await client.getResponse(deleteIndex);
  console.log(responseAfterDelete);
}

// Entry point guard equivalent to Python's `if __name__ == '__main__'`.
if (require.main === module) {
  main().catch((err: unknown) => {
    if (err instanceof AxiosError) {
      console.error('Network/HTTP error:', err.message);
    } else {
      console.error('Unexpected error:', err);
    }
    process.exitCode = 1;
  });
}
