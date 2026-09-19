'use client';
import { Suspense, useCallback, useEffect, useState } from 'react';
import { useProctorAlertStore } from '@/stores/proctorAlertStore';
import { DataTable, type Column } from '@/components/DataTable';
import { Pagination } from '@/components/Pagination';
import { ProctorAlertBadge } from '@/components/StatusBadge';
import { Modal } from '@/components/Modal';
import { formatDateTime, proctorEventTypeText, proctorStatsText } from '@/utils/format';
import type { ProctorAlert } from '@/types';

function ProctorAlerts() {
  const { list, total, loading, fetchList, handle } = useProctorAlertStore();
  const [page, setPage] = useState(1);
  const [status, setStatus] = useState('');
  const [target, setTarget] = useState<ProctorAlert | null>(null);
  const [action, setAction] = useState<'accept' | 'reject'>('accept');
  const [note, setNote] = useState('');
  const [submitting, setSubmitting] = useState(false);

  const reload = useCallback(() => {
    fetchList({ status: status || undefined, page, page_size: 10 });
  }, [fetchList, status, page]);

  useEffect(() => {
    reload();
  }, [reload]);

  const openHandle = (a: ProctorAlert, act: 'accept' | 'reject') => {
    setTarget(a);
    setAction(act);
    setNote('');
  };

  const onSubmitHandle = async () => {
    if (!target) return;
    if (!note.trim()) {
      alert('请填写处理意见（必填）');
      return;
    }
    setSubmitting(true);
    try {
      await handle(target.id, action, note.trim());
      setTarget(null);
      reload();
    } catch (err) {
      alert((err as Error).message);
      // 并发处理冲突时刷新列表，展示最终状态
      reload();
    } finally {
      setSubmitting(false);
    }
  };

  const columns: Column<ProctorAlert>[] = [
    { key: 'exam_title', title: '考试', render: (a) => <span className="font-medium">{a.exam_title}</span> },
    { key: 'student_name', title: '学生', render: (a) => <span>{a.student_name}</span> },
    { key: 'trigger_type', title: '触发事件', render: (a) => (
        <span className="text-red-600">{proctorEventTypeText(a.trigger_type)} × {a.trigger_count}</span>
      ) },
    { key: 'type_stats', title: '事件次数', render: (a) => <span className="text-xs">{proctorStatsText(a.type_stats)}</span> },
    { key: 'status', title: '告警状态', render: (a) => <ProctorAlertBadge status={a.status} /> },
    { key: 'handler_name', title: '处理人', render: (a) => <span>{a.handler_name || '-'}</span> },
    { key: 'handle_note', title: '处理意见', render: (a) => <span className="text-xs">{a.handle_note || '-'}</span> },
    { key: 'handled_at', title: '处理时间', render: (a) => <span className="text-xs">{a.handled_at ? formatDateTime(a.handled_at) : '-'}</span> },
    { key: 'created_at', title: '告警时间', render: (a) => <span className="text-xs">{formatDateTime(a.created_at)}</span> },
    { key: 'actions', title: '操作', render: (a) => (
        a.status === 'pending' ? (
          <div className="flex gap-2">
            <button onClick={() => openHandle(a, 'accept')} className="text-red-600 hover:underline">受理</button>
            <button onClick={() => openHandle(a, 'reject')} className="text-green-600 hover:underline">驳回</button>
          </div>
        ) : (
          <span className="text-xs text-gray-400">已处理</span>
        )
      ) },
  ];

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="text-xl font-bold text-gray-800">监考告警</h1>
        <select value={status} onChange={(e) => { setStatus(e.target.value); setPage(1); }}
          className="rounded-lg border border-gray-300 px-2 py-1.5 text-sm">
          <option value="">全部状态</option>
          <option value="pending">待处理</option>
          <option value="accepted">已受理</option>
          <option value="rejected">已驳回</option>
        </select>
      </div>
      <DataTable columns={columns} rows={list} loading={loading} emptyTitle="暂无监考告警" />
      <Pagination page={page} pageSize={10} total={total} onChange={setPage} />

      <Modal open={!!target} title={action === 'accept' ? '受理告警' : '驳回告警'} onClose={() => setTarget(null)} width="max-w-md">
        {target && (
          <div className="space-y-3">
            <p className="text-sm text-gray-600">
              {target.exam_title} · {target.student_name} · 触发事件 {proctorEventTypeText(target.trigger_type)} × {target.trigger_count}
            </p>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={3}
              maxLength={500}
              placeholder="处理意见（必填）"
              className="w-full rounded-lg border border-gray-300 px-3 py-2 text-sm"
            />
            <div className="flex justify-end gap-3">
              <button onClick={() => setTarget(null)}
                className="rounded-lg border border-gray-300 px-4 py-2 text-sm text-gray-700 hover:bg-gray-50">取消</button>
              <button onClick={onSubmitHandle} disabled={submitting || !note.trim()}
                className={`rounded-lg px-4 py-2 text-sm text-white disabled:opacity-60 ${action === 'accept' ? 'bg-red-600 hover:bg-red-700' : 'bg-green-600 hover:bg-green-700'}`}>
                {submitting ? '提交中…' : action === 'accept' ? '确认受理' : '确认驳回'}
              </button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}

export default function ProctorAlertsPage() {
  return (
    <Suspense fallback={<div className="p-10 text-center text-gray-400">加载中…</div>}>
      <ProctorAlerts />
    </Suspense>
  );
}
