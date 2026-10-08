import { keepPreviousData, QueryClient, useMutation, useQuery, useQueryClient, type QueryKey } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api, ApiError } from '../api.ts'
import type {
  Account, Attempt, AuditEvent, Board, Experiment, ExperimentDetail, Overview, Profile, ProfileVersionInfo, Project,
  Settings, Statistics, Task, TaskVersion, TaskVersionInfo, Trial, Usage, Artifact,
} from './types.ts'

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 2_000,
      refetchOnWindowFocus: true,
      retry: (count, err) => !(err instanceof ApiError && [401, 403, 404, 422].includes(err.status)) && count < 2,
    },
  },
})

export const get = <T,>(path: string) => () => api.request<T>('GET', path)

/** Poll fast while something is moving, slowly otherwise. */
const live = (active: boolean) => (active ? 2_000 : 15_000)

export function useOverview() {
  return useQuery({
    queryKey: ['overview'],
    queryFn: get<Overview>('/api/v1/overview'),
    refetchInterval: (q) => live(!!q.state.data && (q.state.data.running > 0 || q.state.data.queued > 0)),
  })
}

export function useExperiments() {
  return useQuery({
    queryKey: ['experiments'],
    queryFn: get<{ items: Experiment[] | null }>('/api/v1/experiments'),
    select: (d) => d.items ?? [],
    refetchInterval: (q) => live((q.state.data?.items ?? []).some((e) => !['completed', 'cancelled', 'aborted'].includes(e.state))),
  })
}

export function useExperiment(id: string) {
  return useQuery({
    queryKey: ['experiment', id],
    queryFn: get<ExperimentDetail>(`/api/v1/experiments/${id}`),
    enabled: !!id,
    refetchInterval: (q) => live((q.state.data?.summary?.incomplete ?? 0) > 0),
  })
}

export function useTrial(id: string) {
  return useQuery({
    queryKey: ['trial', id],
    queryFn: get<{ trial: Trial; attempts: Attempt[] | null }>(`/api/v1/trials/${id}`),
    enabled: !!id,
    refetchInterval: (q) => live(!!q.state.data && !['completed', 'cancelled', 'aborted'].includes(q.state.data.trial.execution_state)),
  })
}

export function useAttemptChecks(id?: string) {
  return useQuery({ queryKey: ['checks', id], queryFn: get<{ items: string[] | null; note: string }>(`/api/v1/attempts/${id}/checks`), enabled: !!id })
}
export function useAttemptUsage(id?: string) {
  return useQuery({ queryKey: ['usage', id], queryFn: get<Usage>(`/api/v1/attempts/${id}/usage`), enabled: !!id })
}
export function useAttemptArtifacts(id?: string) {
  return useQuery({ queryKey: ['artifacts', id], queryFn: get<{ items: Artifact[] | null }>(`/api/v1/attempts/${id}/artifacts`), enabled: !!id, select: (d) => d.items ?? [] })
}
export function useAttemptReviews(id?: string) {
  return useQuery({ queryKey: ['reviews', id], queryFn: get<{ items: string[] | null; rubrics: string[] | null; note: string }>(`/api/v1/attempts/${id}/reviews`), enabled: !!id })
}

export function useProjects() {
  return useQuery({ queryKey: ['projects'], queryFn: get<{ items: Project[] | null }>('/api/v1/projects?limit=200'), select: (d) => d.items ?? [] })
}
export function useTasks(projectId?: string) {
  return useQuery({ queryKey: ['tasks', projectId], queryFn: get<{ items: Task[] | null }>(`/api/v1/projects/${projectId}/tasks`), enabled: !!projectId, select: (d) => d.items ?? [] })
}
export function useTaskVersions(taskId?: string) {
  return useQuery({ queryKey: ['task-versions', taskId], queryFn: get<{ items: TaskVersion[] | null }>(`/api/v1/tasks/${taskId}/versions`), enabled: !!taskId, select: (d) => d.items ?? [] })
}

export function useCatalog() {
  return useQuery({ queryKey: ['catalog'], queryFn: get<{ tasks: TaskVersionInfo[]; profiles: ProfileVersionInfo[] }>('/api/v1/catalog') })
}

export function useAccounts() {
  return useQuery({ queryKey: ['accounts'], queryFn: get<{ items: Account[] | null }>('/api/v1/accounts'), select: (d) => d.items ?? [] })
}
export function useProfiles() {
  return useQuery({ queryKey: ['profiles'], queryFn: get<{ items: Profile[] | null; latest: Record<string, ProfileVersionInfo> }>('/api/v1/profiles') })
}

export function useComparison(experimentId?: string) {
  return useQuery({
    queryKey: ['comparison', experimentId],
    queryFn: get<{ boards: Board[] | null; statistics: Statistics; excluded_flaky: string[] }>(`/api/v1/comparisons?experiment_id=${experimentId}`),
    enabled: !!experimentId,
    placeholderData: keepPreviousData,
  })
}

export function useSettings() {
  return useQuery({ queryKey: ['settings'], queryFn: get<Settings>('/api/v1/settings') })
}
export function useAudit(enabled: boolean) {
  return useQuery({ queryKey: ['audit'], queryFn: get<{ items: AuditEvent[] | null }>('/api/v1/audit?limit=100'), enabled, select: (d) => d.items ?? [] })
}

/**
 * useAction wraps a mutation with a success toast, an error toast that shows
 * the server message, and cache invalidation.
 */
export function useAction<TVars, TData = unknown>(fn: (vars: TVars) => Promise<TData>, opts: { success?: string | ((data: TData) => string); invalidate?: QueryKey[]; onSuccess?: (data: TData, vars: TVars) => void } = {}) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (data, vars) => {
      if (opts.success) toast.success(typeof opts.success === 'function' ? opts.success(data) : opts.success)
      for (const key of opts.invalidate ?? []) void qc.invalidateQueries({ queryKey: key })
      opts.onSuccess?.(data, vars)
    },
    onError: (err) => {
      if (err instanceof ApiError && err.status === 401) return
      toast.error(errorMessage(err), { description: err instanceof ApiError && err.requestId ? `请求 ${err.requestId}` : undefined })
    },
  })
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 0) return '无法连接到控制面，请确认 agentlab serve 正在运行'
    return err.message || `请求失败（${err.status}）`
  }
  return err instanceof Error ? err.message : '请求失败'
}
