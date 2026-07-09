import { api } from './client';
import type { SettingsGroups, SettingsSection } from '@/types/admin';

export const settingsApi = {
  get: () => api.get<SettingsGroups>('/settings'),
  update: (section: SettingsSection, value: Record<string, unknown>) =>
    api.put<Record<string, unknown>>(`/settings/${section}`, value),
};
