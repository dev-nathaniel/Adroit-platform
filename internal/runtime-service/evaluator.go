package runtimeservice

import (
	"fmt"
	"math"
	"time"

	"github.com/dev-nathaniel/Adroit-platform/pkg/types"
)

// PolicyEvaluator computes scaling decisions for a workload
type PolicyEvaluator struct{}

// NewPolicyEvaluator creates an instance of PolicyEvaluator
func NewPolicyEvaluator() *PolicyEvaluator {
	return &PolicyEvaluator{}
}

// Evaluate determines whether a workload should scale up or down based on metrics
func (e *PolicyEvaluator) Evaluate(
	policy *types.ServiceAutoScalingPolicy,
	metrics *types.LiveServiceMetrics,
	now time.Time,
) ScalingDecision {
	if !policy.AutoScalingEnabled {
		return ScalingDecision{
			ShouldScale:      false,
			PreviousReplicas: policy.CurrentReplicas,
			DesiredReplicas:  policy.CurrentReplicas,
			Reason:           "autoscaling disabled on service",
		}
	}

	current := policy.CurrentReplicas
	if current < 1 {
		current = 1
	}

	minReplicas := policy.MinReplicas
	if minReplicas < 1 {
		minReplicas = 1
	}
	maxReplicas := policy.MaxReplicas
	if maxReplicas < minReplicas {
		maxReplicas = minReplicas
	}

	// 1. Calculate recommendations per trigger metric
	var highestDesired int = current
	var dominantReason string
	var triggerMetric string
	var metricVal float64
	var targetVal float64

	// A. CPU-based horizontal evaluation
	if policy.TargetCPUPercent > 0 && metrics.CPUPercent > 0 {
		target := float64(policy.TargetCPUPercent)
		ratio := metrics.CPUPercent / target

		// 10% deadband / tolerance to suppress oscillation
		if ratio > 1.10 || ratio < 0.85 {
			desiredCPU := int(math.Ceil(float64(current) * ratio))
			if desiredCPU != current {
				highestDesired = desiredCPU
				triggerMetric = "cpu"
				metricVal = metrics.CPUPercent
				targetVal = target
				if desiredCPU > current {
					dominantReason = fmt.Sprintf("CPU utilization (%.1f%%) exceeded target threshold (%d%%)", metrics.CPUPercent, policy.TargetCPUPercent)
				} else {
					dominantReason = fmt.Sprintf("CPU utilization (%.1f%%) dropped below target threshold (%d%%)", metrics.CPUPercent, policy.TargetCPUPercent)
				}
			}
		}
	}

	// B. Memory-based horizontal evaluation
	if policy.TargetMemoryPercent > 0 && metrics.MemoryPercent > 0 {
		target := float64(policy.TargetMemoryPercent)
		ratio := metrics.MemoryPercent / target

		if ratio > 1.10 {
			desiredMem := int(math.Ceil(float64(current) * ratio))
			if desiredMem > highestDesired {
				highestDesired = desiredMem
				triggerMetric = "memory"
				metricVal = metrics.MemoryPercent
				targetVal = target
				dominantReason = fmt.Sprintf("Memory usage (%.1f%%) exceeded target threshold (%d%%)", metrics.MemoryPercent, policy.TargetMemoryPercent)
			}
		}
	}

	// C. Traffic / RPS-based horizontal evaluation
	if policy.TargetRPS > 0 && metrics.RequestsPerSec > 0 {
		target := float64(policy.TargetRPS)
		desiredRPS := int(math.Ceil(metrics.RequestsPerSec / target))
		if desiredRPS > highestDesired {
			highestDesired = desiredRPS
			triggerMetric = "rps"
			metricVal = metrics.RequestsPerSec
			targetVal = target
			dominantReason = fmt.Sprintf("Request rate (%.1f RPS) exceeded capacity threshold (%d RPS/instance)", metrics.RequestsPerSec, policy.TargetRPS)
		}
	}

	// Clamp recommendation to configured elastic boundaries [min, max]
	desired := highestDesired
	if desired < minReplicas {
		desired = minReplicas
		if current != desired {
			dominantReason = fmt.Sprintf("Scaled down to configured min_replicas limit (%d)", minReplicas)
			if triggerMetric == "" {
				triggerMetric = "boundary_min"
			}
		}
	}
	if desired > maxReplicas {
		desired = maxReplicas
		dominantReason = fmt.Sprintf("Scaling capped at configured max_replicas limit (%d)", maxReplicas)
	}

	// If desired count matches current active count, no scaling action is needed
	if desired == current {
		return ScalingDecision{
			ShouldScale:      false,
			PreviousReplicas: current,
			DesiredReplicas:  current,
			Reason:           "current replicas optimal for measured traffic",
		}
	}

	// 2. Enforce stabilization windows (scale-up and scale-down cooldowns)
	scaleUpCooldown := time.Duration(policy.ScaleUpCooldownSeconds) * time.Second
	if scaleUpCooldown <= 0 {
		scaleUpCooldown = 30 * time.Second
	}
	scaleDownCooldown := time.Duration(policy.ScaleDownCooldownSeconds) * time.Second
	if scaleDownCooldown <= 0 {
		scaleDownCooldown = 180 * time.Second
	}

	if policy.LastScaledAt != nil {
		elapsed := now.Sub(*policy.LastScaledAt)
		if desired > current && elapsed < scaleUpCooldown {
			return ScalingDecision{
				ShouldScale:      false,
				PreviousReplicas: current,
				DesiredReplicas:  desired,
				Reason:           fmt.Sprintf("scale-up suppressed by stabilization window (%vs remaining)", int((scaleUpCooldown-elapsed).Seconds())),
				TriggerMetric:    triggerMetric,
				MetricValue:      metricVal,
				TargetValue:      targetVal,
				CooldownSkipped:  true,
			}
		}
		if desired < current && elapsed < scaleDownCooldown {
			return ScalingDecision{
				ShouldScale:      false,
				PreviousReplicas: current,
				DesiredReplicas:  desired,
				Reason:           fmt.Sprintf("scale-down suppressed by stabilization window (%vs remaining)", int((scaleDownCooldown-elapsed).Seconds())),
				TriggerMetric:    triggerMetric,
				MetricValue:      metricVal,
				TargetValue:      targetVal,
				CooldownSkipped:  true,
			}
		}
	}

	return ScalingDecision{
		ShouldScale:      true,
		PreviousReplicas: current,
		DesiredReplicas:  desired,
		Reason:           dominantReason,
		TriggerMetric:    triggerMetric,
		MetricValue:      metricVal,
		TargetValue:      targetVal,
	}
}
