import { createHash } from 'node:crypto';

/**
 * Deterministic offline assist provider for Promptfoo.
 * Mirrors Pathfinder OfflineAssistProvider: never calls a model API.
 */
export default class OfflineAssistProvider {
  constructor(options = {}) {
    this.providerId = options.id || 'pathfinder-offline-assist';
    this.config = options.config || {};
  }

  id() {
    return this.providerId;
  }

  async callApi(prompt) {
    const digest = createHash('sha256').update(String(prompt)).digest('hex').slice(0, 16);
    const output = {
      normalized_statement: 'offline assist replay — review required',
      modality: 'must',
      suggested_pattern_id: 'manual_review',
      rationale:
        'Deterministic offline fixture response for Promptfoo/CI; not a live model judgment.',
      fixture_digest: digest,
      executable: false,
      approval_status: 'pending_review',
    };
    return { output: JSON.stringify(output) };
  }
}
