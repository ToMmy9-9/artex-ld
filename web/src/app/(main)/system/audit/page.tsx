/**
 * page.tsx：审计日志页——仅渲染 AuditRecords 组件。
 * 主要函数/类型：
 *   - AuditPage：默认导出页组件
 */
import { AuditRecords } from "@/components/audit-records";

export default function AuditPage() {
  return (
    <div className="flex min-w-0 flex-1 flex-col p-4 sm:p-6">
      <AuditRecords />
    </div>
  );
}
