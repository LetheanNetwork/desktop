// SPDX-Licence-Identifier: EUPL-1.2

//go:build darwin && cgo

package telemetry

/*
#cgo LDFLAGS: -framework CoreFoundation -framework IOKit

#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/ps/IOPowerSources.h>
#include <IOKit/ps/IOPSKeys.h>
#include <ifaddrs.h>
#include <mach/mach.h>
#include <net/if.h>
#include <net/if_dl.h>
#include <net/if_var.h>
#include <stdint.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/sysctl.h>

typedef struct {
    int available;
    int source;
    int has_battery_percent;
    double battery_percent;
    int has_charging;
    int charging;
} lthn_power_info;

static int lthn_read_cpu(uint64_t *user, uint64_t *system, uint64_t *nice, uint64_t *idle) {
    host_cpu_load_info_data_t info;
    mach_msg_type_number_t count = HOST_CPU_LOAD_INFO_COUNT;
    mach_port_t host = mach_host_self();
    kern_return_t result = host_statistics(
        host,
        HOST_CPU_LOAD_INFO,
        (host_info_t)&info,
        &count
    );
    mach_port_deallocate(mach_task_self(), host);
    if (result != KERN_SUCCESS) {
        return (int)result;
    }
    *user = (uint64_t)info.cpu_ticks[CPU_STATE_USER];
    *system = (uint64_t)info.cpu_ticks[CPU_STATE_SYSTEM];
    *nice = (uint64_t)info.cpu_ticks[CPU_STATE_NICE];
    *idle = (uint64_t)info.cpu_ticks[CPU_STATE_IDLE];
    return 0;
}

static int lthn_read_memory(uint64_t *total, uint64_t *used) {
    size_t total_size = sizeof(*total);
    if (sysctlbyname("hw.memsize", total, &total_size, NULL, 0) != 0) {
        return -1;
    }

    vm_size_t page_size = 0;
    mach_port_t host = mach_host_self();
    kern_return_t page_result = host_page_size(host, &page_size);
    if (page_result != KERN_SUCCESS) {
        mach_port_deallocate(mach_task_self(), host);
        return -2;
    }

    vm_statistics64_data_t stats;
    mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
    kern_return_t statistics_result = host_statistics64(
        host,
        HOST_VM_INFO64,
        (host_info64_t)&stats,
        &count
    );
    mach_port_deallocate(mach_task_self(), host);
    if (statistics_result != KERN_SUCCESS) {
        return -3;
    }

    uint64_t used_pages = (uint64_t)stats.active_count
        + (uint64_t)stats.wire_count
        + (uint64_t)stats.compressor_page_count;
    uint64_t calculated = used_pages * (uint64_t)page_size;
    *used = calculated > *total ? *total : calculated;
    return 0;
}

static int lthn_read_network(uint64_t *received, uint64_t *sent) {
    struct ifaddrs *interfaces = NULL;
    if (getifaddrs(&interfaces) != 0) {
        return -1;
    }

    uint64_t total_received = 0;
    uint64_t total_sent = 0;
    for (struct ifaddrs *current = interfaces; current != NULL; current = current->ifa_next) {
        if (current->ifa_addr == NULL || current->ifa_data == NULL) {
            continue;
        }
        if (current->ifa_addr->sa_family != AF_LINK || (current->ifa_flags & IFF_LOOPBACK) != 0) {
            continue;
        }
        const struct if_data *data = (const struct if_data *)current->ifa_data;
        total_received += (uint64_t)data->ifi_ibytes;
        total_sent += (uint64_t)data->ifi_obytes;
    }
    freeifaddrs(interfaces);
    *received = total_received;
    *sent = total_sent;
    return 0;
}

static int lthn_cfnumber_int(CFDictionaryRef dictionary, CFStringRef key, int *value) {
    CFTypeRef raw = CFDictionaryGetValue(dictionary, key);
    if (raw == NULL || CFGetTypeID(raw) != CFNumberGetTypeID()) {
        return 0;
    }
    return CFNumberGetValue((CFNumberRef)raw, kCFNumberIntType, value) ? 1 : 0;
}

static int lthn_read_power(lthn_power_info *output) {
    memset(output, 0, sizeof(*output));
    CFTypeRef snapshot = IOPSCopyPowerSourcesInfo();
    if (snapshot == NULL) {
        return -1;
    }

    output->available = 1;
    CFStringRef providing = IOPSGetProvidingPowerSourceType(snapshot);
    if (providing != NULL && CFEqual(providing, CFSTR(kIOPSACPowerValue))) {
        output->source = 1;
    } else if (providing != NULL && CFEqual(providing, CFSTR(kIOPSBatteryPowerValue))) {
        output->source = 2;
    }

    CFArrayRef sources = IOPSCopyPowerSourcesList(snapshot);
    if (sources != NULL && CFArrayGetCount(sources) > 0) {
        CFTypeRef source = CFArrayGetValueAtIndex(sources, 0);
        CFDictionaryRef description = IOPSGetPowerSourceDescription(snapshot, source);
        if (description != NULL) {
            int current = 0;
            int maximum = 0;
            if (lthn_cfnumber_int(description, CFSTR(kIOPSCurrentCapacityKey), &current)
                && lthn_cfnumber_int(description, CFSTR(kIOPSMaxCapacityKey), &maximum)
                && maximum > 0) {
                output->has_battery_percent = 1;
                output->battery_percent = ((double)current / (double)maximum) * 100.0;
            }
            CFTypeRef charging = CFDictionaryGetValue(description, CFSTR(kIOPSIsChargingKey));
            if (charging != NULL && CFGetTypeID(charging) == CFBooleanGetTypeID()) {
                output->has_charging = 1;
                output->charging = CFBooleanGetValue((CFBooleanRef)charging) ? 1 : 0;
            }
        }
    }

    if (sources != NULL) {
        CFRelease(sources);
    }
    CFRelease(snapshot);
    return 0;
}
*/
import "C"

import core "dappco.re/go"

type darwinHostSource struct{}

func newPlatformHostSource() hostSource {
	return darwinHostSource{}
}

func (darwinHostSource) read() (hostReading, error) {
	reading := hostReading{
		observedAt:   core.Now(),
		source:       "macOS host APIs",
		platform:     core.OS(),
		architecture: core.Arch(),
		logicalCores: core.NumCPU(),
	}

	var user, system, nice, idle C.uint64_t
	if result := C.lthn_read_cpu(&user, &system, &nice, &idle); result != 0 {
		return hostReading{}, core.NewError("telemetry: macOS CPU counters are unavailable")
	}
	reading.cpu = &cpuCounters{
		user:   uint64(user),
		system: uint64(system),
		nice:   uint64(nice),
		idle:   uint64(idle),
	}

	var total, used C.uint64_t
	if result := C.lthn_read_memory(&total, &used); result != 0 {
		return hostReading{}, core.NewError("telemetry: macOS memory counters are unavailable")
	}
	reading.memory = &memoryReading{
		totalBytes: uint64(total),
		usedBytes:  uint64(used),
	}

	var received, sent C.uint64_t
	if result := C.lthn_read_network(&received, &sent); result != 0 {
		return hostReading{}, core.NewError("telemetry: macOS network counters are unavailable")
	}
	reading.network = &networkCounters{
		receivedBytes: uint64(received),
		sentBytes:     uint64(sent),
	}

	var nativePower C.lthn_power_info
	if result := C.lthn_read_power(&nativePower); result == 0 && nativePower.available != 0 {
		power := &powerReading{source: PowerSourceUnknown}
		switch nativePower.source {
		case 1:
			power.source = PowerSourceAC
		case 2:
			power.source = PowerSourceBattery
		}
		if nativePower.has_battery_percent != 0 {
			percentage := float64(nativePower.battery_percent)
			power.batteryPercent = &percentage
		}
		if nativePower.has_charging != 0 {
			charging := nativePower.charging != 0
			power.charging = &charging
		}
		reading.power = power
	}

	return reading, nil
}
