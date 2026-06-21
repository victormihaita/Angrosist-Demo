import { useQuery } from '@tanstack/react-query'
import {
  listLeads,
  getLeadDetail,
  listUsers,
  ApiError,
  type LeadFilters,
} from '@/lib/api'

/** Paginated, filtered lead list. Filters/cursor come from the URL. */
export function useLeadsList(filters: LeadFilters) {
  return useQuery({
    queryKey: ['leads', filters],
    queryFn: () => listLeads(filters),
    // Keep the previous page visible while the next loads (smooth pagination).
    placeholderData: (prev) => prev,
  })
}

/** Full lead detail (transcript + company + contact + offer). */
export function useLeadDetail(id: string) {
  return useQuery({
    queryKey: ['lead', id],
    queryFn: () => getLeadDetail(id),
    enabled: !!id,
  })
}

/**
 * Dashboard users for the assignee picker. Admin-only on the backend; if it 403s
 * for staff we resolve to an empty list so the UI can degrade (assign-to-self /
 * hide picker) rather than error.
 */
export function useUsers() {
  return useQuery({
    queryKey: ['users'],
    queryFn: async () => {
      try {
        return await listUsers()
      } catch (err) {
        if (err instanceof ApiError && err.status === 403) return []
        throw err
      }
    },
    staleTime: 5 * 60_000,
  })
}
