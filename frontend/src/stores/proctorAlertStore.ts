// 监考告警状态（教师告警列表与受理/驳回复用）。
import { create } from 'zustand';
import { proctorAlertApi } from '@/api/proctorAlert';
import type { ProctorAlert } from '@/types';

interface ProctorAlertState {
  list: ProctorAlert[];
  total: number;
  loading: boolean;
  fetchList: (query: { status?: string; exam_id?: string; page?: number; page_size?: number }) => Promise<void>;
  handle: (id: string, action: 'accept' | 'reject', note: string) => Promise<ProctorAlert>;
}

export const useProctorAlertStore = create<ProctorAlertState>((set) => ({
  list: [],
  total: 0,
  loading: false,
  fetchList: async (query) => {
    set({ loading: true });
    try {
      const res = await proctorAlertApi.list(query);
      set({ list: res.list, total: res.total });
    } finally {
      set({ loading: false });
    }
  },
  handle: (id, action, note) => proctorAlertApi.handle(id, action, note),
}));
