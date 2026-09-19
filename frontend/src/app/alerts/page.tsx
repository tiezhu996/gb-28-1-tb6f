'use client';
import { useCallback, useEffect, useState } from 'react';
import { useProctorStore } from '@/stores/proctorStore';
import { proctorApi } from '@/api/proctor';
import { DataTable, type Column } from '@/components/DataTable';
import { Pagination } from '@/components/Pagination';
import { Modal } from '@/components/Modal';
import { AlertStatusBadge } from '@/components/StatusBadge';
import { formatDateTime, proctorEventTypeText } from '@/utils/format';
import { ALERT_ACTIONS, ALERT_STATUS } from '@/constants';
import type { ProctorAlert, ProctorAlertDetail } from '@/types';

const STATUS_TABS = [
  { value: '', label: '全部' },
  { value: ALERT_STATUS.PENDING, label: '待处理' },
  { value: ALERT_STATUS.CONFIRMED, label: '已受理' },
  { value: ALERT_STATUS.REJECTED, label: '已驳回' },
];

export default function ProctorAlertsPage() {
  const { alerts, total, loading, fetchAlerts, handle } = useProctorStore();
  const [status, setStatus] = useState('');
  const [page, setPage] = useState(1);
  const [current, setCurrent] = useState<ProctorAlert | null>(null);
  const [detail, setDetail] = useState<ProctorAlertDetail | null>(null);
  const [action, setAction] = useState<'confirm' | 'reject'>(ALERT_ACTIONS.CONFIRM);
  const [opinion, setOpinion] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const reload = useCallback(() => {
    fetchAlerts({ status: status || undefined, page, page_size: 10 });
  }, [fetchAlerts, status, page]);

  useEffect(() => {
    reload();
  }, [reload]);

  const openDetail = async (item: ProctorAlert) => {
    setCurrent(item);
    setAction(ALERT_ACTIONS.CONFIRM);
    setOpinion('');
    setDetail(null);
    try {
      setDetail(await proctorApi.detail(item.id));
    } catch (err) {
      alert((err as Error).message);
    }
  };

  const closeModal = () => {
    setCurrent(null);
    setDetail(null);
  };

  const onHandle = async () => {
    if (!current) return;
    if (!opinion.trim()) {
      alert('处理意见必填');
      return;
    }
    setSubmitting(true);
    try {
      await handle(current.id, action, opinion.trim());
      closeModal();
      reload();
    } catch (err) {
      // 并发处理冲突等错误：提示后刷新列表，展示最终状态
      alert((err as Error).message);
      reload();
    } finally {
      setSubmitting(false);
    }
  };

  const columns: Column<ProctorAlert>[] = [
    { key: 'student_name', title: '学生', render: (a) => <span className="font-medium">{a.student_name}</span> },
    { key: 'exam_title', title: '考试', render: (a) => <span>{a.exam_title}</span> },
    { key: 'trigger_type', title: '触发类型', render: (a) => <span>{proctorEventTypeText(a.trigger_type)}</span> },
    {
      key: 'counts', title: '违规次数', render: (a) => (
        <span className="text-red-600">切屏 {a.switch_count} · 粘贴 {a.paste_count}</span>
      ),
    },
    { key: 'status', title: '告警状态', render: (a) => <AlertStatusBadge status={a.status} /> },
    {
      key: 'handled', title: '处理信息', render: (a) => (
        a.handled_at
          ? <span className="text-xs">{a.handler_name} · {formatDateTime(a.handled_at)}</span>
          : <span className="text-xs text-gray-400">-</span>
      ),
    },
    { key: 'created_at', title: '告警时间', render: (a) => <span className="text-xs">{formatDateTime(a.created_at)}</span> },
    {
      key: 'actions', title: '操作', render: (a) => (
        <button onClick={() => openDetail(a)} className="text-brand-600 hover:underline">
          {a.status === ALERT_STATUS.PENDING ? '处理' : '查看'}
        </button>
      ),
    },
  ];

  const pending = current?.status === ALERT_STATUS.PENDING;

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-bold text-gray-800">监考告警处理</h1>
        <div className="flex gap-1 rounded-lg bg-gray-100 p-1">
          {STATUS_TABS.map((t) => (
            <button
              key={t.value}
              onClick={() => { setStatus(t.value); setPage(1); }}
              className={`rounded-md px-3 py-1 text-sm ${status === t.value ? 'bg-white font-medium text-gray-800 shadow-sm' : 'text-gray-500'}`}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      <DataTable columns={columns} rows={alerts} loading={loading} emptyTitle="暂无监考告警" />
      <Pagination page={page} pageSize={10} total={total} onChange={setPage} />

      <Modal open={!!current} title={pending ? '处理监考告警' : '监考告警详情'} onClose={closeModal} width="max-w-2xl">
        {current && (
          <div className="space-y-4">
            <div className="grid gap-2 text-sm sm:grid-cols-2">
              <p><span className="text-gray-500">学生：</span>{current.student_name}</p>
              <p><span className="text-gray-500">考试：</span>{current.exam_title}</p>
              <p><span className="text-gray-500">触发类型：</span>{proctorEventTypeText(current.trigger_type)}</p>
              <p>
                <span className="text-gray-500">状态：</span>
                <AlertStatusBadge status={current.status} />
              </p>
              <p className="text-red-600">切屏 {current.switch_count} 次 · 粘贴 {current.paste_count} 次</p>
              <p className="text-gray-400">告警时间 {formatDateTime(current.created_at)}</p>
            </div>

            <div>
              <h4 className="mb-2 text-sm font-semibold text-gray-700">事件留痕</h4>
              {!detail ? (
                <p className="text-xs text-gray-400">加载中…</p>
              ) : detail.events.length === 0 ? (
                <p className="text-xs text-gray-400">暂无留痕</p>
              ) : (
                <ul className="max-h-48 space-y-1 overflow-y-auto rounded-lg bg-gray-50 p-3 text-xs">
                  {detail.events.map((e) => (
                    <li key={e.id} className="flex items-center justify-between gap-2">
                      <span>
                        {proctorEventTypeText(e.type)}
                        {e.count > 1 && <span className="ml-1 text-red-500">×{e.count}（连续累计）</span>}
                        {e.detail && <span className="ml-1 text-gray-400">{e.detail}</span>}
                      </span>
                      <span className="text-gray-400">{formatDateTime(e.first_at)} ~ {formatDateTime(e.last_at)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            {pending ? (
              <div className="space-y-3 border-t border-gray-100 pt-3">
                <div className="flex gap-4 text-sm">
                  <label className="flex items-center gap-1.5">
                    <input type="radio" name="alert-action" checked={action === ALERT_ACTIONS.CONFIRM} onChange={() => setAction(ALERT_ACTIONS.CONFIRM)} />
                    受理（确认违规）
                  </label>
                  <label className="flex items-center gap-1.5">
                    <input type="radio" name="alert-action" checked={action === ALERT_ACTIONS.REJECT} onChange={() => setAction(ALERT_ACTIONS.REJECT)} />
                    驳回（误报）
                  </label>
                </div>
                <textarea
                  value={opinion}
                  onChange={(e) => setOpinion(e.target.value)}
                  rows={3}
                  maxLength={500}
                  placeholder="处理意见（必填）"
                  className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
                />
                <div className="flex justify-end gap-2">
                  <button onClick={closeModal} className="rounded-lg border border-gray-300 px-4 py-2 text-sm hover:bg-gray-50">取消</button>
                  <button
                    onClick={onHandle}
                    disabled={submitting || !opinion.trim()}
                    className="rounded-lg bg-brand-600 px-4 py-2 text-sm text-white hover:bg-brand-700 disabled:opacity-60"
                  >
                    {submitting ? '提交中…' : '提交处理结果'}
                  </button>
                </div>
              </div>
            ) : (
              <div className="space-y-1 rounded-lg bg-gray-50 p-3 text-sm">
                <p><span className="text-gray-500">处理结论：</span>{current.status === ALERT_STATUS.CONFIRMED ? '已受理（确认违规）' : '已驳回'}</p>
                <p><span className="text-gray-500">处理意见：</span>{current.opinion}</p>
                <p><span className="text-gray-500">处理人：</span>{current.handler_name} · {formatDateTime(current.handled_at)}</p>
              </div>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
