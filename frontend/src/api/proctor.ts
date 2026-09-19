import { request, buildQuery } from '@/utils/request';
import type { PageResult, ProctorAlert, ProctorAlertDetail, ProctorReportResult } from '@/types';

export interface ReportProctorEventInput {
  type: 'switch_tab' | 'paste';
  detail?: string;
}

export const proctorApi = {
  // 学生：答题时上报切屏/粘贴事件（每次单独留痕，连续同类只累计次数）
  reportEvent(recordId: string, input: ReportProctorEventInput) {
    return request<ProctorReportResult>(`/exam-records/${recordId}/proctor-events`, {
      method: 'POST',
      body: JSON.stringify(input),
    });
  },
  // 教师：告警列表
  list(query: { status?: string; exam_id?: string; page?: number; page_size?: number }) {
    return request<PageResult<ProctorAlert>>(`/proctor-alerts${buildQuery({ ...query })}`);
  },
  // 教师：告警详情（含该答卷全部事件留痕）
  detail(id: string) {
    return request<ProctorAlertDetail>(`/proctor-alerts/${id}`);
  },
  // 教师：用自己的账号受理/驳回（处理意见必填，并发只成功一次）
  handle(id: string, action: 'confirm' | 'reject', opinion: string) {
    return request<ProctorAlert>(`/proctor-alerts/${id}/handle`, {
      method: 'POST',
      body: JSON.stringify({ action, opinion }),
    });
  },
};
