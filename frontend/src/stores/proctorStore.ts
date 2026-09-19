// 监考告警状态（教师告警处理页复用）。
import { create } from 'zustand';
import { proctorApi } from '@/api/proctor';
import type { ProctorAlert } from '@/types';

interface ProctorState {
  alerts: ProctorAlert[];
  total: number;
  loading: boolean;
  fetchAlerts: (query: { status?: string; exam_id?: string; page?: number; page_size?: number }) => Promise<void>;
  handle: (id: string, action: 'confirm' | 'reject', opinion: string) => Promise<ProctorAlert>;
}

export const useProctorStore = create<ProctorState>((set) => ({
  alerts: [],
  total: 0,
  loading: false,
  fetchAlerts: async (query) => {
    set({ loading: true });
    try {
      const res = await proctorApi.list(query);
      set({ alerts: res.list, total: res.total });
    } finally {
      set({ loading: false });
    }
  },
  handle: (id, action, opinion) => proctorApi.handle(id, action, opinion),
}));
