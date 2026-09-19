import { request, buildQuery } from '@/utils/request';
import type { PageResult, ProctorAlert } from '@/types';

export const proctorAlertApi = {
  list(query: { status?: string; exam_id?: string; page?: number; page_size?: number }) {
    return request<PageResult<ProctorAlert>>(`/proctor-alerts${buildQuery({ ...query })}`);
  },
  get(id: string) {
    return request<ProctorAlert>(`/proctor-alerts/${id}`);
  },
  // 教师受理/驳回（处理人取自登录账号，处理意见必填）
  handle(id: string, action: 'accept' | 'reject', note: string) {
    return request<ProctorAlert>(`/proctor-alerts/${id}/handle`, {
      method: 'POST',
      body: JSON.stringify({ action, note }),
    });
  },
};
