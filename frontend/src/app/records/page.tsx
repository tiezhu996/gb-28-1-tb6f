'use client';
import { useCallback, useEffect, useState } from 'react';
import Link from 'next/link';
import { useRecordStore } from '@/stores/recordStore';
import { DataTable, type Column } from '@/components/DataTable';
import { Pagination } from '@/components/Pagination';
import { ProctorAlertBadge, StatusBadge } from '@/components/StatusBadge';
import { formatDateTime, proctorStatsText, recordStatusColor, recordStatusText } from '@/utils/format';
import type { ExamRecord } from '@/types';

export default function RecordsPage() {
  const { mine, total, loading, fetchMine } = useRecordStore();
  const [page, setPage] = useState(1);

  const reload = useCallback(() => {
    fetchMine({ page, page_size: 10 });
  }, [fetchMine, page]);

  useEffect(() => {
    reload();
  }, [reload]);

  const columns: Column<ExamRecord>[] = [
    { key: 'exam_title', title: '考试', render: (r) => <span className="font-medium">{r.exam_title}</span> },
    { key: 'status', title: '状态', render: (r) => <StatusBadge text={recordStatusText(r.status)} color={recordStatusColor(r.status)} /> },
    { key: 'objective_score', title: '客观题分', render: (r) => <span>{r.objective_score}</span> },
    { key: 'final_score', title: '最终分', render: (r) => <span className="font-semibold">{r.final_score || '-'}</span> },
    { key: 'proctor_stats', title: '监考事件', render: (r) => (
        <span className={proctorStatsText(r.proctor_stats) !== '0' ? 'text-red-600' : 'text-gray-500'}>
          {proctorStatsText(r.proctor_stats)}
        </span>
      ) },
    { key: 'alert_status', title: '告警状态', render: (r) => <ProctorAlertBadge status={r.alert_status} /> },
    { key: 'started_at', title: '开始时间', render: (r) => <span className="text-xs">{formatDateTime(r.started_at)}</span> },
    { key: 'actions', title: '操作', render: (r) => (
        <div className="flex gap-2">
          {r.status === 'in_progress' ? (
            <Link href={`/exam-take?recordId=${r.id}`} className="text-brand-600 hover:underline">继续作答</Link>
          ) : (
            <Link href={`/records/review?recordId=${r.id}`} className="text-brand-600 hover:underline">查看答卷</Link>
          )}
        </div>
      ) },
  ];

  return (
    <div className="space-y-4">
      <h1 className="text-xl font-bold text-gray-800">我的考试记录</h1>
      <DataTable columns={columns} rows={mine} loading={loading} emptyTitle="还没有参加过考试" />
      <Pagination page={page} pageSize={10} total={total} onChange={setPage} />
    </div>
  );
}
