import { describe, expect, it } from 'vitest'

import {
  canResumeImageRun,
  imageTaskFromRun,
  isImageExecutor,
} from '../imageStatus'
import type { ApplicationRun } from '../types'

function imageRun(status: string, runStatus = 'running'): ApplicationRun {
  return {
    id: `run-${status}`,
    app_slug: 'free-media-studio',
    status: runStatus,
    next_action: {
      step_key: 'media',
      executor: 'image_generation',
      credential_role: 'media',
      api_key_id: 1,
      session_id: 'app:image-run',
      path: '/v1/images/generations',
      method: 'POST',
      content_type: 'application/json',
      request: {},
      body: {},
      gateway_request_id: 'image-123',
    },
    steps: [{
      step_key: 'media',
      executor: 'image_generation',
      status: status === 'processing' ? 'running' : status,
      gateway_request_id: 'image-123',
      output: { task_id: 'image-123', status },
    }],
  }
}

describe('application image task lifecycle', () => {
  it('recognizes only image executors', () => {
    expect(isImageExecutor('image_generation')).toBe(true)
    expect(isImageExecutor('image_edit')).toBe(true)
    expect(isImageExecutor('video_generation')).toBe(false)
  })

  it('restores a processing task and keeps it resumable', () => {
    const run = imageRun('processing')

    expect(imageTaskFromRun(run)).toMatchObject({
      taskId: 'image-123',
      status: 'processing',
      stepKey: 'media',
    })
    expect(canResumeImageRun(run)).toBe(true)
  })

  it.each(['failed', 'completed', 'cancelled'])('does not resume terminal %s tasks', (status) => {
    expect(canResumeImageRun(imageRun(status, status))).toBe(false)
  })

  it('lets a cancelled run override a stale processing checkpoint', () => {
    const run = imageRun('processing', 'cancelled')

    expect(imageTaskFromRun(run)?.status).toBe('cancelled')
    expect(canResumeImageRun(run)).toBe(false)
  })

  it('uses the persisted step task when next_action is absent', () => {
    const run = imageRun('processing')
    run.next_action = null

    expect(imageTaskFromRun(run)?.taskId).toBe('image-123')
  })
})
