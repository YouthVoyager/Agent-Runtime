import { api } from './client';
import type { Page } from '@/types/common';
import type { Artifact } from '@/types/task';

export interface ListArtifactsParams {
  task_id?: string;
  type?: string;
  name?: string;
  created_from?: string;
  created_to?: string;
  page?: number;
  page_size?: number;
}

export interface ArtifactDetail extends Artifact {
  metadata?: Record<string, unknown>;
  content?: unknown;
  download_url?: string;
}

export const artifactsApi = {
  list: (params: ListArtifactsParams) => api.get<Page<Artifact>>('/artifacts', params as Record<string, string>),
  get: (artifactId: string) => api.get<ArtifactDetail>(`/artifacts/${artifactId}`),
  downloadUrl: (artifactId: string) => `/api/v1/artifacts/${artifactId}/download`,
  remove: (artifactId: string) => api.delete<void>(`/artifacts/${artifactId}`),
};
