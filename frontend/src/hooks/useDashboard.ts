import { useQuery } from '@tanstack/react-query'
import {
  listLeads,
  getLeadDetail,
  listUsers,
  listCompanies,
  getCompany,
  listHandoffs,
  getKpis,
  ApiError,
  type LeadFilters,
  type CompanyFilters,
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

/** Paginated, filtered B2B directory list. Filters/cursor come from the URL. */
export function useCompaniesList(filters: CompanyFilters) {
  return useQuery({
    queryKey: ['companies', filters],
    queryFn: () => listCompanies(filters),
    placeholderData: (prev) => prev,
  })
}

/** Full company detail (identity + verification + financials). */
export function useCompanyDetail(id: string) {
  return useQuery({
    queryKey: ['company', id],
    queryFn: () => getCompany(id),
    enabled: !!id,
  })
}

/** Human-handoff queue (needs_human leads). */
export function useHandoffs() {
  return useQuery({
    queryKey: ['handoffs'],
    queryFn: () => listHandoffs(),
  })
}

/** Dashboard KPI aggregates. Admin-gated on the backend; 403 → null so the
 *  KPI strip simply hides for staff instead of erroring. */
export function useKpis() {
  return useQuery({
    queryKey: ['kpis'],
    queryFn: async () => {
      try {
        return await getKpis()
      } catch (err) {
        if (err instanceof ApiError && err.status === 403) return null
        throw err
      }
    },
    staleTime: 60_000,
  })
}
