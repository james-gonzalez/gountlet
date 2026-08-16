//go:build cgo

// Package gpu benchmarks GPU compute throughput by dispatching a Vulkan
// compute shader that performs a large number of fused multiply-adds and
// timing the dispatch. The Vulkan loader (libvulkan.so.1 / vulkan-1.dll /
// libvulkan.dylib) is loaded dynamically at runtime (dlopen/LoadLibrary),
// not linked at build time, so a machine without it installed can still
// run gountlet for every other benchmark instead of failing to start at
// all. The Vulkan headers are vendored under vk/include (see vk/LICENSE)
// so building needs only a C compiler, no Vulkan SDK, on any platform. No
// display or windowing system is required to run the benchmark itself.
package gpu

/*
#cgo CFLAGS: -I${SRCDIR}/vk/include
#cgo linux LDFLAGS: -ldl
#define VK_NO_PROTOTYPES
#include <vulkan/vulkan_core.h>
#include <stdlib.h>
#include <string.h>

#ifdef _WIN32
#include <windows.h>
typedef HMODULE gountlet_lib_t;
static gountlet_lib_t gountlet_dlopen(void) {
	return LoadLibraryA("vulkan-1.dll");
}
static void *gountlet_dlsym(gountlet_lib_t lib, const char *name) {
	return (void *)GetProcAddress(lib, name);
}
static void gountlet_dlclose(gountlet_lib_t lib) {
	FreeLibrary(lib);
}
#else
#include <dlfcn.h>
typedef void *gountlet_lib_t;
static gountlet_lib_t gountlet_dlopen(void) {
#ifdef __APPLE__
	gountlet_lib_t h = dlopen("libvulkan.dylib", RTLD_NOW | RTLD_LOCAL);
	if (!h) h = dlopen("libMoltenVK.dylib", RTLD_NOW | RTLD_LOCAL);
	return h;
#else
	return dlopen("libvulkan.so.1", RTLD_NOW | RTLD_LOCAL);
#endif
}
static void *gountlet_dlsym(gountlet_lib_t lib, const char *name) {
	return dlsym(lib, name);
}
static void gountlet_dlclose(gountlet_lib_t lib) {
	dlclose(lib);
}
#endif

// ---- pfn storage ----
static PFN_vkGetInstanceProcAddr pfn_vkGetInstanceProcAddr;
static PFN_vkCreateInstance pfn_vkCreateInstance;
static PFN_vkEnumerateInstanceExtensionProperties pfn_vkEnumerateInstanceExtensionProperties;
static PFN_vkDestroyInstance pfn_vkDestroyInstance;
static PFN_vkEnumeratePhysicalDevices pfn_vkEnumeratePhysicalDevices;
static PFN_vkGetPhysicalDeviceProperties pfn_vkGetPhysicalDeviceProperties;
static PFN_vkGetPhysicalDeviceQueueFamilyProperties pfn_vkGetPhysicalDeviceQueueFamilyProperties;
static PFN_vkGetPhysicalDeviceMemoryProperties pfn_vkGetPhysicalDeviceMemoryProperties;
static PFN_vkCreateDevice pfn_vkCreateDevice;
static PFN_vkGetDeviceProcAddr pfn_vkGetDeviceProcAddr;
static PFN_vkGetDeviceQueue pfn_vkGetDeviceQueue;
static PFN_vkDestroyDevice pfn_vkDestroyDevice;
static PFN_vkCreateBuffer pfn_vkCreateBuffer;
static PFN_vkDestroyBuffer pfn_vkDestroyBuffer;
static PFN_vkGetBufferMemoryRequirements pfn_vkGetBufferMemoryRequirements;
static PFN_vkAllocateMemory pfn_vkAllocateMemory;
static PFN_vkFreeMemory pfn_vkFreeMemory;
static PFN_vkBindBufferMemory pfn_vkBindBufferMemory;
static PFN_vkMapMemory pfn_vkMapMemory;
static PFN_vkUnmapMemory pfn_vkUnmapMemory;
static PFN_vkCreateShaderModule pfn_vkCreateShaderModule;
static PFN_vkDestroyShaderModule pfn_vkDestroyShaderModule;
static PFN_vkCreateDescriptorSetLayout pfn_vkCreateDescriptorSetLayout;
static PFN_vkDestroyDescriptorSetLayout pfn_vkDestroyDescriptorSetLayout;
static PFN_vkCreatePipelineLayout pfn_vkCreatePipelineLayout;
static PFN_vkDestroyPipelineLayout pfn_vkDestroyPipelineLayout;
static PFN_vkCreateComputePipelines pfn_vkCreateComputePipelines;
static PFN_vkDestroyPipeline pfn_vkDestroyPipeline;
static PFN_vkCreateDescriptorPool pfn_vkCreateDescriptorPool;
static PFN_vkDestroyDescriptorPool pfn_vkDestroyDescriptorPool;
static PFN_vkAllocateDescriptorSets pfn_vkAllocateDescriptorSets;
static PFN_vkUpdateDescriptorSets pfn_vkUpdateDescriptorSets;
static PFN_vkCreateCommandPool pfn_vkCreateCommandPool;
static PFN_vkDestroyCommandPool pfn_vkDestroyCommandPool;
static PFN_vkAllocateCommandBuffers pfn_vkAllocateCommandBuffers;
static PFN_vkCreateFence pfn_vkCreateFence;
static PFN_vkDestroyFence pfn_vkDestroyFence;
static PFN_vkCreateQueryPool pfn_vkCreateQueryPool;
static PFN_vkDestroyQueryPool pfn_vkDestroyQueryPool;
static PFN_vkResetCommandBuffer pfn_vkResetCommandBuffer;
static PFN_vkResetFences pfn_vkResetFences;
static PFN_vkBeginCommandBuffer pfn_vkBeginCommandBuffer;
static PFN_vkEndCommandBuffer pfn_vkEndCommandBuffer;
static PFN_vkCmdResetQueryPool pfn_vkCmdResetQueryPool;
static PFN_vkCmdWriteTimestamp pfn_vkCmdWriteTimestamp;
static PFN_vkCmdBindPipeline pfn_vkCmdBindPipeline;
static PFN_vkCmdBindDescriptorSets pfn_vkCmdBindDescriptorSets;
static PFN_vkCmdPushConstants pfn_vkCmdPushConstants;
static PFN_vkCmdDispatch pfn_vkCmdDispatch;
static PFN_vkQueueSubmit pfn_vkQueueSubmit;
static PFN_vkWaitForFences pfn_vkWaitForFences;
static PFN_vkGetQueryPoolResults pfn_vkGetQueryPoolResults;

// ---- trampolines (same names as the real Vulkan functions, so the rest
// of this file and the Go code below need no changes at all) ----
static inline VkResult vkCreateInstance(const VkInstanceCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkInstance* pInstance) {
	return pfn_vkCreateInstance(pCreateInfo, pAllocator, pInstance);
}
static inline VkResult vkEnumerateInstanceExtensionProperties(const char* pLayerName, uint32_t* pPropertyCount, VkExtensionProperties* pProperties) {
	return pfn_vkEnumerateInstanceExtensionProperties(pLayerName, pPropertyCount, pProperties);
}
static inline void vkDestroyInstance(VkInstance instance, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyInstance(instance, pAllocator);
}
static inline VkResult vkEnumeratePhysicalDevices(VkInstance instance, uint32_t* pPhysicalDeviceCount, VkPhysicalDevice* pPhysicalDevices) {
	return pfn_vkEnumeratePhysicalDevices(instance, pPhysicalDeviceCount, pPhysicalDevices);
}
static inline void vkGetPhysicalDeviceProperties(VkPhysicalDevice physicalDevice, VkPhysicalDeviceProperties* pProperties) {
	pfn_vkGetPhysicalDeviceProperties(physicalDevice, pProperties);
}
static inline void vkGetPhysicalDeviceQueueFamilyProperties(VkPhysicalDevice physicalDevice, uint32_t* pQueueFamilyPropertyCount, VkQueueFamilyProperties* pQueueFamilyProperties) {
	pfn_vkGetPhysicalDeviceQueueFamilyProperties(physicalDevice, pQueueFamilyPropertyCount, pQueueFamilyProperties);
}
static inline void vkGetPhysicalDeviceMemoryProperties(VkPhysicalDevice physicalDevice, VkPhysicalDeviceMemoryProperties* pMemoryProperties) {
	pfn_vkGetPhysicalDeviceMemoryProperties(physicalDevice, pMemoryProperties);
}
static inline VkResult vkCreateDevice(VkPhysicalDevice physicalDevice, const VkDeviceCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkDevice* pDevice) {
	return pfn_vkCreateDevice(physicalDevice, pCreateInfo, pAllocator, pDevice);
}
static inline PFN_vkVoidFunction vkGetDeviceProcAddr(VkDevice device, const char* pName) {
	return pfn_vkGetDeviceProcAddr(device, pName);
}
static inline void vkGetDeviceQueue(VkDevice device, uint32_t queueFamilyIndex, uint32_t queueIndex, VkQueue* pQueue) {
	pfn_vkGetDeviceQueue(device, queueFamilyIndex, queueIndex, pQueue);
}
static inline void vkDestroyDevice(VkDevice device, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyDevice(device, pAllocator);
}
static inline VkResult vkCreateBuffer(VkDevice device, const VkBufferCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkBuffer* pBuffer) {
	return pfn_vkCreateBuffer(device, pCreateInfo, pAllocator, pBuffer);
}
static inline void vkDestroyBuffer(VkDevice device, VkBuffer buffer, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyBuffer(device, buffer, pAllocator);
}
static inline void vkGetBufferMemoryRequirements(VkDevice device, VkBuffer buffer, VkMemoryRequirements* pMemoryRequirements) {
	pfn_vkGetBufferMemoryRequirements(device, buffer, pMemoryRequirements);
}
static inline VkResult vkAllocateMemory(VkDevice device, const VkMemoryAllocateInfo* pAllocateInfo, const VkAllocationCallbacks* pAllocator, VkDeviceMemory* pMemory) {
	return pfn_vkAllocateMemory(device, pAllocateInfo, pAllocator, pMemory);
}
static inline void vkFreeMemory(VkDevice device, VkDeviceMemory memory, const VkAllocationCallbacks* pAllocator) {
	pfn_vkFreeMemory(device, memory, pAllocator);
}
static inline VkResult vkBindBufferMemory(VkDevice device, VkBuffer buffer, VkDeviceMemory memory, VkDeviceSize memoryOffset) {
	return pfn_vkBindBufferMemory(device, buffer, memory, memoryOffset);
}
static inline VkResult vkMapMemory(VkDevice device, VkDeviceMemory memory, VkDeviceSize offset, VkDeviceSize size, VkMemoryMapFlags flags, void** ppData) {
	return pfn_vkMapMemory(device, memory, offset, size, flags, ppData);
}
static inline void vkUnmapMemory(VkDevice device, VkDeviceMemory memory) {
	pfn_vkUnmapMemory(device, memory);
}
static inline VkResult vkCreateShaderModule(VkDevice device, const VkShaderModuleCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkShaderModule* pShaderModule) {
	return pfn_vkCreateShaderModule(device, pCreateInfo, pAllocator, pShaderModule);
}
static inline void vkDestroyShaderModule(VkDevice device, VkShaderModule shaderModule, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyShaderModule(device, shaderModule, pAllocator);
}
static inline VkResult vkCreateDescriptorSetLayout(VkDevice device, const VkDescriptorSetLayoutCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkDescriptorSetLayout* pSetLayout) {
	return pfn_vkCreateDescriptorSetLayout(device, pCreateInfo, pAllocator, pSetLayout);
}
static inline void vkDestroyDescriptorSetLayout(VkDevice device, VkDescriptorSetLayout descriptorSetLayout, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyDescriptorSetLayout(device, descriptorSetLayout, pAllocator);
}
static inline VkResult vkCreatePipelineLayout(VkDevice device, const VkPipelineLayoutCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkPipelineLayout* pPipelineLayout) {
	return pfn_vkCreatePipelineLayout(device, pCreateInfo, pAllocator, pPipelineLayout);
}
static inline void vkDestroyPipelineLayout(VkDevice device, VkPipelineLayout pipelineLayout, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyPipelineLayout(device, pipelineLayout, pAllocator);
}
static inline VkResult vkCreateComputePipelines(VkDevice device, VkPipelineCache pipelineCache, uint32_t createInfoCount, const VkComputePipelineCreateInfo* pCreateInfos, const VkAllocationCallbacks* pAllocator, VkPipeline* pPipelines) {
	return pfn_vkCreateComputePipelines(device, pipelineCache, createInfoCount, pCreateInfos, pAllocator, pPipelines);
}
static inline void vkDestroyPipeline(VkDevice device, VkPipeline pipeline, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyPipeline(device, pipeline, pAllocator);
}
static inline VkResult vkCreateDescriptorPool(VkDevice device, const VkDescriptorPoolCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkDescriptorPool* pDescriptorPool) {
	return pfn_vkCreateDescriptorPool(device, pCreateInfo, pAllocator, pDescriptorPool);
}
static inline void vkDestroyDescriptorPool(VkDevice device, VkDescriptorPool descriptorPool, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyDescriptorPool(device, descriptorPool, pAllocator);
}
static inline VkResult vkAllocateDescriptorSets(VkDevice device, const VkDescriptorSetAllocateInfo* pAllocateInfo, VkDescriptorSet* pDescriptorSets) {
	return pfn_vkAllocateDescriptorSets(device, pAllocateInfo, pDescriptorSets);
}
static inline void vkUpdateDescriptorSets(VkDevice device, uint32_t descriptorWriteCount, const VkWriteDescriptorSet* pDescriptorWrites, uint32_t descriptorCopyCount, const VkCopyDescriptorSet* pDescriptorCopies) {
	pfn_vkUpdateDescriptorSets(device, descriptorWriteCount, pDescriptorWrites, descriptorCopyCount, pDescriptorCopies);
}
static inline VkResult vkCreateCommandPool(VkDevice device, const VkCommandPoolCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkCommandPool* pCommandPool) {
	return pfn_vkCreateCommandPool(device, pCreateInfo, pAllocator, pCommandPool);
}
static inline void vkDestroyCommandPool(VkDevice device, VkCommandPool commandPool, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyCommandPool(device, commandPool, pAllocator);
}
static inline VkResult vkAllocateCommandBuffers(VkDevice device, const VkCommandBufferAllocateInfo* pAllocateInfo, VkCommandBuffer* pCommandBuffers) {
	return pfn_vkAllocateCommandBuffers(device, pAllocateInfo, pCommandBuffers);
}
static inline VkResult vkCreateFence(VkDevice device, const VkFenceCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkFence* pFence) {
	return pfn_vkCreateFence(device, pCreateInfo, pAllocator, pFence);
}
static inline void vkDestroyFence(VkDevice device, VkFence fence, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyFence(device, fence, pAllocator);
}
static inline VkResult vkCreateQueryPool(VkDevice device, const VkQueryPoolCreateInfo* pCreateInfo, const VkAllocationCallbacks* pAllocator, VkQueryPool* pQueryPool) {
	return pfn_vkCreateQueryPool(device, pCreateInfo, pAllocator, pQueryPool);
}
static inline void vkDestroyQueryPool(VkDevice device, VkQueryPool queryPool, const VkAllocationCallbacks* pAllocator) {
	pfn_vkDestroyQueryPool(device, queryPool, pAllocator);
}
static inline VkResult vkResetCommandBuffer(VkCommandBuffer commandBuffer, VkCommandBufferResetFlags flags) {
	return pfn_vkResetCommandBuffer(commandBuffer, flags);
}
static inline VkResult vkResetFences(VkDevice device, uint32_t fenceCount, const VkFence* pFences) {
	return pfn_vkResetFences(device, fenceCount, pFences);
}
static inline VkResult vkBeginCommandBuffer(VkCommandBuffer commandBuffer, const VkCommandBufferBeginInfo* pBeginInfo) {
	return pfn_vkBeginCommandBuffer(commandBuffer, pBeginInfo);
}
static inline VkResult vkEndCommandBuffer(VkCommandBuffer commandBuffer) {
	return pfn_vkEndCommandBuffer(commandBuffer);
}
static inline void vkCmdResetQueryPool(VkCommandBuffer commandBuffer, VkQueryPool queryPool, uint32_t firstQuery, uint32_t queryCount) {
	pfn_vkCmdResetQueryPool(commandBuffer, queryPool, firstQuery, queryCount);
}
static inline void vkCmdWriteTimestamp(VkCommandBuffer commandBuffer, VkPipelineStageFlagBits pipelineStage, VkQueryPool queryPool, uint32_t query) {
	pfn_vkCmdWriteTimestamp(commandBuffer, pipelineStage, queryPool, query);
}
static inline void vkCmdBindPipeline(VkCommandBuffer commandBuffer, VkPipelineBindPoint pipelineBindPoint, VkPipeline pipeline) {
	pfn_vkCmdBindPipeline(commandBuffer, pipelineBindPoint, pipeline);
}
static inline void vkCmdBindDescriptorSets(VkCommandBuffer commandBuffer, VkPipelineBindPoint pipelineBindPoint, VkPipelineLayout layout, uint32_t firstSet, uint32_t descriptorSetCount, const VkDescriptorSet* pDescriptorSets, uint32_t dynamicOffsetCount, const uint32_t* pDynamicOffsets) {
	pfn_vkCmdBindDescriptorSets(commandBuffer, pipelineBindPoint, layout, firstSet, descriptorSetCount, pDescriptorSets, dynamicOffsetCount, pDynamicOffsets);
}
static inline void vkCmdPushConstants(VkCommandBuffer commandBuffer, VkPipelineLayout layout, VkShaderStageFlags stageFlags, uint32_t offset, uint32_t size, const void* pValues) {
	pfn_vkCmdPushConstants(commandBuffer, layout, stageFlags, offset, size, pValues);
}
static inline void vkCmdDispatch(VkCommandBuffer commandBuffer, uint32_t groupCountX, uint32_t groupCountY, uint32_t groupCountZ) {
	pfn_vkCmdDispatch(commandBuffer, groupCountX, groupCountY, groupCountZ);
}
static inline VkResult vkQueueSubmit(VkQueue queue, uint32_t submitCount, const VkSubmitInfo* pSubmits, VkFence fence) {
	return pfn_vkQueueSubmit(queue, submitCount, pSubmits, fence);
}
static inline VkResult vkWaitForFences(VkDevice device, uint32_t fenceCount, const VkFence* pFences, VkBool32 waitAll, uint64_t timeout) {
	return pfn_vkWaitForFences(device, fenceCount, pFences, waitAll, timeout);
}
static inline VkResult vkGetQueryPoolResults(VkDevice device, VkQueryPool queryPool, uint32_t firstQuery, uint32_t queryCount, size_t dataSize, void* pData, VkDeviceSize stride, VkQueryResultFlags flags) {
	return pfn_vkGetQueryPoolResults(device, queryPool, firstQuery, queryCount, dataSize, pData, stride, flags);
}

// gountlet_vk_load_global resolves the handful of functions usable before
// any VkInstance exists. Returns 1 on success.
static int gountlet_vk_load_global(void) {
	pfn_vkCreateInstance = (PFN_vkCreateInstance)pfn_vkGetInstanceProcAddr(NULL, "vkCreateInstance");
	pfn_vkEnumerateInstanceExtensionProperties = (PFN_vkEnumerateInstanceExtensionProperties)pfn_vkGetInstanceProcAddr(NULL, "vkEnumerateInstanceExtensionProperties");
	return pfn_vkCreateInstance && pfn_vkEnumerateInstanceExtensionProperties;
}

// gountlet_vk_load_instance resolves every function reachable once an
// instance exists, including vkGetDeviceProcAddr itself.
static void gountlet_vk_load_instance(VkInstance instance) {
	pfn_vkDestroyInstance = (PFN_vkDestroyInstance)pfn_vkGetInstanceProcAddr(instance, "vkDestroyInstance");
	pfn_vkEnumeratePhysicalDevices = (PFN_vkEnumeratePhysicalDevices)pfn_vkGetInstanceProcAddr(instance, "vkEnumeratePhysicalDevices");
	pfn_vkGetPhysicalDeviceProperties = (PFN_vkGetPhysicalDeviceProperties)pfn_vkGetInstanceProcAddr(instance, "vkGetPhysicalDeviceProperties");
	pfn_vkGetPhysicalDeviceQueueFamilyProperties = (PFN_vkGetPhysicalDeviceQueueFamilyProperties)pfn_vkGetInstanceProcAddr(instance, "vkGetPhysicalDeviceQueueFamilyProperties");
	pfn_vkGetPhysicalDeviceMemoryProperties = (PFN_vkGetPhysicalDeviceMemoryProperties)pfn_vkGetInstanceProcAddr(instance, "vkGetPhysicalDeviceMemoryProperties");
	pfn_vkCreateDevice = (PFN_vkCreateDevice)pfn_vkGetInstanceProcAddr(instance, "vkCreateDevice");
	pfn_vkGetDeviceProcAddr = (PFN_vkGetDeviceProcAddr)pfn_vkGetInstanceProcAddr(instance, "vkGetDeviceProcAddr");
}

// gountlet_vk_load_device resolves every device-level function once a
// VkDevice exists.
static void gountlet_vk_load_device(VkDevice device) {
	pfn_vkGetDeviceQueue = (PFN_vkGetDeviceQueue)pfn_vkGetDeviceProcAddr(device, "vkGetDeviceQueue");
	pfn_vkDestroyDevice = (PFN_vkDestroyDevice)pfn_vkGetDeviceProcAddr(device, "vkDestroyDevice");
	pfn_vkCreateBuffer = (PFN_vkCreateBuffer)pfn_vkGetDeviceProcAddr(device, "vkCreateBuffer");
	pfn_vkDestroyBuffer = (PFN_vkDestroyBuffer)pfn_vkGetDeviceProcAddr(device, "vkDestroyBuffer");
	pfn_vkGetBufferMemoryRequirements = (PFN_vkGetBufferMemoryRequirements)pfn_vkGetDeviceProcAddr(device, "vkGetBufferMemoryRequirements");
	pfn_vkAllocateMemory = (PFN_vkAllocateMemory)pfn_vkGetDeviceProcAddr(device, "vkAllocateMemory");
	pfn_vkFreeMemory = (PFN_vkFreeMemory)pfn_vkGetDeviceProcAddr(device, "vkFreeMemory");
	pfn_vkBindBufferMemory = (PFN_vkBindBufferMemory)pfn_vkGetDeviceProcAddr(device, "vkBindBufferMemory");
	pfn_vkMapMemory = (PFN_vkMapMemory)pfn_vkGetDeviceProcAddr(device, "vkMapMemory");
	pfn_vkUnmapMemory = (PFN_vkUnmapMemory)pfn_vkGetDeviceProcAddr(device, "vkUnmapMemory");
	pfn_vkCreateShaderModule = (PFN_vkCreateShaderModule)pfn_vkGetDeviceProcAddr(device, "vkCreateShaderModule");
	pfn_vkDestroyShaderModule = (PFN_vkDestroyShaderModule)pfn_vkGetDeviceProcAddr(device, "vkDestroyShaderModule");
	pfn_vkCreateDescriptorSetLayout = (PFN_vkCreateDescriptorSetLayout)pfn_vkGetDeviceProcAddr(device, "vkCreateDescriptorSetLayout");
	pfn_vkDestroyDescriptorSetLayout = (PFN_vkDestroyDescriptorSetLayout)pfn_vkGetDeviceProcAddr(device, "vkDestroyDescriptorSetLayout");
	pfn_vkCreatePipelineLayout = (PFN_vkCreatePipelineLayout)pfn_vkGetDeviceProcAddr(device, "vkCreatePipelineLayout");
	pfn_vkDestroyPipelineLayout = (PFN_vkDestroyPipelineLayout)pfn_vkGetDeviceProcAddr(device, "vkDestroyPipelineLayout");
	pfn_vkCreateComputePipelines = (PFN_vkCreateComputePipelines)pfn_vkGetDeviceProcAddr(device, "vkCreateComputePipelines");
	pfn_vkDestroyPipeline = (PFN_vkDestroyPipeline)pfn_vkGetDeviceProcAddr(device, "vkDestroyPipeline");
	pfn_vkCreateDescriptorPool = (PFN_vkCreateDescriptorPool)pfn_vkGetDeviceProcAddr(device, "vkCreateDescriptorPool");
	pfn_vkDestroyDescriptorPool = (PFN_vkDestroyDescriptorPool)pfn_vkGetDeviceProcAddr(device, "vkDestroyDescriptorPool");
	pfn_vkAllocateDescriptorSets = (PFN_vkAllocateDescriptorSets)pfn_vkGetDeviceProcAddr(device, "vkAllocateDescriptorSets");
	pfn_vkUpdateDescriptorSets = (PFN_vkUpdateDescriptorSets)pfn_vkGetDeviceProcAddr(device, "vkUpdateDescriptorSets");
	pfn_vkCreateCommandPool = (PFN_vkCreateCommandPool)pfn_vkGetDeviceProcAddr(device, "vkCreateCommandPool");
	pfn_vkDestroyCommandPool = (PFN_vkDestroyCommandPool)pfn_vkGetDeviceProcAddr(device, "vkDestroyCommandPool");
	pfn_vkAllocateCommandBuffers = (PFN_vkAllocateCommandBuffers)pfn_vkGetDeviceProcAddr(device, "vkAllocateCommandBuffers");
	pfn_vkCreateFence = (PFN_vkCreateFence)pfn_vkGetDeviceProcAddr(device, "vkCreateFence");
	pfn_vkDestroyFence = (PFN_vkDestroyFence)pfn_vkGetDeviceProcAddr(device, "vkDestroyFence");
	pfn_vkCreateQueryPool = (PFN_vkCreateQueryPool)pfn_vkGetDeviceProcAddr(device, "vkCreateQueryPool");
	pfn_vkDestroyQueryPool = (PFN_vkDestroyQueryPool)pfn_vkGetDeviceProcAddr(device, "vkDestroyQueryPool");
	pfn_vkResetCommandBuffer = (PFN_vkResetCommandBuffer)pfn_vkGetDeviceProcAddr(device, "vkResetCommandBuffer");
	pfn_vkResetFences = (PFN_vkResetFences)pfn_vkGetDeviceProcAddr(device, "vkResetFences");
	pfn_vkBeginCommandBuffer = (PFN_vkBeginCommandBuffer)pfn_vkGetDeviceProcAddr(device, "vkBeginCommandBuffer");
	pfn_vkEndCommandBuffer = (PFN_vkEndCommandBuffer)pfn_vkGetDeviceProcAddr(device, "vkEndCommandBuffer");
	pfn_vkCmdResetQueryPool = (PFN_vkCmdResetQueryPool)pfn_vkGetDeviceProcAddr(device, "vkCmdResetQueryPool");
	pfn_vkCmdWriteTimestamp = (PFN_vkCmdWriteTimestamp)pfn_vkGetDeviceProcAddr(device, "vkCmdWriteTimestamp");
	pfn_vkCmdBindPipeline = (PFN_vkCmdBindPipeline)pfn_vkGetDeviceProcAddr(device, "vkCmdBindPipeline");
	pfn_vkCmdBindDescriptorSets = (PFN_vkCmdBindDescriptorSets)pfn_vkGetDeviceProcAddr(device, "vkCmdBindDescriptorSets");
	pfn_vkCmdPushConstants = (PFN_vkCmdPushConstants)pfn_vkGetDeviceProcAddr(device, "vkCmdPushConstants");
	pfn_vkCmdDispatch = (PFN_vkCmdDispatch)pfn_vkGetDeviceProcAddr(device, "vkCmdDispatch");
	pfn_vkQueueSubmit = (PFN_vkQueueSubmit)pfn_vkGetDeviceProcAddr(device, "vkQueueSubmit");
	pfn_vkWaitForFences = (PFN_vkWaitForFences)pfn_vkGetDeviceProcAddr(device, "vkWaitForFences");
	pfn_vkGetQueryPoolResults = (PFN_vkGetQueryPoolResults)pfn_vkGetDeviceProcAddr(device, "vkGetQueryPoolResults");
}

static gountlet_lib_t gountlet_vk_lib;

// gountlet_vk_init dlopen's the platform's Vulkan loader and resolves the
// handful of functions callable before an instance exists. Returns 1 on
// success, 0 if the loader isn't installed on this machine — callers use
// that to report GPU benchmarking as unavailable instead of the whole
// binary refusing to start, which is what linking -lvulkan at build time
// used to cause on any machine without the loader (e.g. a Raspberry Pi
// with no Vulkan install, just trying to run the CPU benchmark).
static int gountlet_vk_init(void) {
	gountlet_vk_lib = gountlet_dlopen();
	if (!gountlet_vk_lib) return 0;
	pfn_vkGetInstanceProcAddr = (PFN_vkGetInstanceProcAddr)gountlet_dlsym(gountlet_vk_lib, "vkGetInstanceProcAddr");
	if (!pfn_vkGetInstanceProcAddr) return 0;
	return gountlet_vk_load_global();
}

static void gountlet_vk_close(void) {
	if (gountlet_vk_lib) {
		gountlet_dlclose(gountlet_vk_lib);
		gountlet_vk_lib = NULL;
	}
}

static VkResult gountlet_create_instance(VkInstance *instance) {
	VkApplicationInfo appInfo;
	memset(&appInfo, 0, sizeof(appInfo));
	appInfo.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO;
	appInfo.pApplicationName = "gountlet";
	appInfo.applicationVersion = 1;
	appInfo.pEngineName = "gountlet";
	appInfo.engineVersion = 1;
	appInfo.apiVersion = VK_API_VERSION_1_0;

	VkInstanceCreateInfo ci;
	memset(&ci, 0, sizeof(ci));
	ci.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO;
	ci.pApplicationInfo = &appInfo;

	// macOS has no native Vulkan driver: MoltenVK translates to Metal, and
	// it's a "portability subset" ICD. Since Vulkan SDK 1.3.216 the loader
	// refuses to even consider such ICDs unless the app opts in via
	// VK_KHR_portability_enumeration + the matching create flag —
	// otherwise vkCreateInstance fails with VK_ERROR_INCOMPATIBLE_DRIVER
	// (-9) despite MoltenVK being installed and working. Only enable it
	// when the loader actually offers it, so this stays a no-op on
	// Linux/Windows where the extension doesn't exist.
	uint32_t extCount = 0;
	vkEnumerateInstanceExtensionProperties(NULL, &extCount, NULL);
	VkExtensionProperties *exts = NULL;
	if (extCount > 0) {
		exts = (VkExtensionProperties *)malloc(sizeof(VkExtensionProperties) * extCount);
		vkEnumerateInstanceExtensionProperties(NULL, &extCount, exts);
	}
	const char *portabilityExt = VK_KHR_PORTABILITY_ENUMERATION_EXTENSION_NAME;
	for (uint32_t i = 0; i < extCount; i++) {
		if (strcmp(exts[i].extensionName, portabilityExt) == 0) {
			ci.flags |= VK_INSTANCE_CREATE_ENUMERATE_PORTABILITY_BIT_KHR;
			ci.enabledExtensionCount = 1;
			ci.ppEnabledExtensionNames = &portabilityExt;
			break;
		}
	}
	free(exts);

	return vkCreateInstance(&ci, NULL, instance);
}
*/
import "C"

import (
	_ "embed"
	"fmt"
	"time"
	"unsafe"

	"github.com/james-gonzalez/gountlet/internal/bench"
)

//go:embed shader.spv
var shaderSPV []byte

const (
	numElements = 1 << 20 // 1,048,576 floats, 4 MiB buffer
	localSizeX  = 256
	flopsPerFMA = 2 // one multiply + one add
	calibIters  = 4000
)

// cnew copies v into newly C.malloc'd memory and returns a pointer to it.
//
// Vulkan's CreateInfo structs are full of pointer-to-struct/array fields
// (pQueueCreateInfos, pBindings, pSetLayouts, ...). If those pointers targeted
// ordinary Go values, the enclosing struct would contain a Go pointer, and
// cgo's runtime pointer checker forbids passing a Go pointer to C whose
// target itself contains further Go pointers. Routing every such target
// through C-owned memory sidesteps the rule entirely instead of fighting it
// field-by-field with runtime.Pinner. Callers must free the returned pointer
// (via freeAll/defer) once the Vulkan call that consumes it has returned.
func cnew[T any](v T) *T {
	p := (*T)(C.malloc(C.size_t(unsafe.Sizeof(v))))
	*p = v
	return p
}

// freeAll frees every pointer allocated by cnew, in any order.
func freeAll(ptrs ...unsafe.Pointer) {
	for _, p := range ptrs {
		C.free(p)
	}
}

// ctx bundles every Vulkan handle the benchmark creates, so cleanup can walk
// through it in reverse creation order regardless of where an error occurs.
type ctx struct {
	instance       C.VkInstance
	device         C.VkDevice
	queue          C.VkQueue
	queueFamily    C.uint32_t
	buffer         C.VkBuffer
	memory         C.VkDeviceMemory
	shaderModule   C.VkShaderModule
	setLayout      C.VkDescriptorSetLayout
	pipelineLayout C.VkPipelineLayout
	pipeline       C.VkPipeline
	descriptorPool C.VkDescriptorPool
	descriptorSet  C.VkDescriptorSet
	commandPool    C.VkCommandPool
	commandBuffer  C.VkCommandBuffer
	fence          C.VkFence
	queryPool      C.VkQueryPool
	deviceName     string
	// timestampPeriod is nanoseconds per timestamp tick; 0 means the
	// device/queue doesn't support timestamp queries, so dispatch falls
	// back to CPU wall-clock timing (which also captures driver/MoltenVK
	// submission overhead, not just GPU execution time).
	timestampPeriod float32
}

// destroy releases every handle that was successfully created, in reverse order.
func (c *ctx) destroy() {
	if c.queryPool != nil {
		C.vkDestroyQueryPool(c.device, c.queryPool, nil)
	}
	if c.fence != nil {
		C.vkDestroyFence(c.device, c.fence, nil)
	}
	if c.commandPool != nil {
		C.vkDestroyCommandPool(c.device, c.commandPool, nil)
	}
	if c.descriptorPool != nil {
		C.vkDestroyDescriptorPool(c.device, c.descriptorPool, nil)
	}
	if c.pipeline != nil {
		C.vkDestroyPipeline(c.device, c.pipeline, nil)
	}
	if c.pipelineLayout != nil {
		C.vkDestroyPipelineLayout(c.device, c.pipelineLayout, nil)
	}
	if c.setLayout != nil {
		C.vkDestroyDescriptorSetLayout(c.device, c.setLayout, nil)
	}
	if c.shaderModule != nil {
		C.vkDestroyShaderModule(c.device, c.shaderModule, nil)
	}
	if c.memory != nil {
		C.vkFreeMemory(c.device, c.memory, nil)
	}
	if c.buffer != nil {
		C.vkDestroyBuffer(c.device, c.buffer, nil)
	}
	if c.device != nil {
		C.vkDestroyDevice(c.device, nil)
	}
	if c.instance != nil {
		C.vkDestroyInstance(c.instance, nil)
	}
}

func vkErr(op string, res C.VkResult) error {
	return fmt.Errorf("%s failed: VkResult %d", op, int32(res))
}

// Run measures GPU compute throughput in GFLOPS using a Vulkan compute
// shader, plus reports the selected device's name. duration is how long
// the (calibrated) compute dispatch runs for.
func Run(duration time.Duration) bench.Result {
	name := "gpu"

	if C.gountlet_vk_init() == 0 {
		return bench.Fail(name, fmt.Errorf("no Vulkan loader found (install libvulkan.so.1 / vulkan-1.dll / libvulkan.dylib to enable GPU benchmarking)"))
	}
	defer C.gountlet_vk_close()

	c := &ctx{}
	defer c.destroy()

	if res := C.gountlet_create_instance(&c.instance); res != C.VK_SUCCESS {
		return bench.Fail(name, vkErr("vkCreateInstance", res))
	}
	C.gountlet_vk_load_instance(c.instance)

	sel, err := pickDevice(c.instance)
	if err != nil {
		return bench.Fail(name, err)
	}
	physDevice := sel.device
	c.queueFamily = sel.queueFamily
	c.deviceName = sel.name
	c.timestampPeriod = sel.timestampPeriod
	vramBytes := deviceLocalMemoryBytes(physDevice)

	if err := c.createDevice(physDevice); err != nil {
		return bench.Fail(name, err)
	}
	if err := c.createBuffer(physDevice); err != nil {
		return bench.Fail(name, err)
	}
	if err := c.createPipeline(); err != nil {
		return bench.Fail(name, err)
	}
	if err := c.createCommands(); err != nil {
		return bench.Fail(name, err)
	}

	// Calibrate: run a small iteration count to estimate iterations/sec,
	// then scale up to hit roughly duration of GPU work.
	calibElapsed, err := c.dispatch(calibIters)
	if err != nil {
		return bench.Fail(name, err)
	}
	itersPerSec := float64(calibIters) / calibElapsed.Seconds()
	targetIters := uint32(itersPerSec * duration.Seconds())
	if targetIters < calibIters {
		targetIters = calibIters
	}

	elapsed, err := c.dispatch(targetIters)
	if err != nil {
		return bench.Fail(name, err)
	}

	totalFlops := float64(numElements) * float64(targetIters) * flopsPerFMA
	gflops := totalFlops / elapsed.Seconds() / 1e9

	deviceKind := "integrated"
	if sel.discrete {
		deviceKind = "discrete"
	}

	res := bench.Result{Name: name}
	res.Add("compute", gflops, "GFLOPS", deviceKind+" GPU")
	res.AddInfo("device", c.deviceName)
	if c.timestampPeriod > 0 {
		res.AddInfo("timing", "GPU timestamp (excludes driver dispatch overhead)")
	} else {
		res.AddInfo("timing", "CPU wall-clock (device has no timestamp query support; includes some driver dispatch overhead)")
	}
	if vramBytes > 0 {
		res.AddInfo("vram", bench.FormatBytes(vramBytes))
	}
	return res
}

// selectedDevice bundles everything Run needs about the chosen physical
// device and its compute queue family.
type selectedDevice struct {
	device          C.VkPhysicalDevice
	queueFamily     C.uint32_t
	name            string
	discrete        bool
	timestampPeriod float32 // nanoseconds per timestamp tick; 0 if the device/queue can't do timestamp queries
}

// pickDevice enumerates physical devices and returns the first discrete GPU
// with a compute-capable queue family, falling back to any such device.
func pickDevice(instance C.VkInstance) (selectedDevice, error) {
	var count C.uint32_t
	if res := C.vkEnumeratePhysicalDevices(instance, &count, nil); res != C.VK_SUCCESS {
		return selectedDevice{}, vkErr("vkEnumeratePhysicalDevices", res)
	}
	if count == 0 {
		return selectedDevice{}, fmt.Errorf("no Vulkan physical devices found")
	}
	devices := make([]C.VkPhysicalDevice, count)
	if res := C.vkEnumeratePhysicalDevices(instance, &count, &devices[0]); res != C.VK_SUCCESS {
		return selectedDevice{}, vkErr("vkEnumeratePhysicalDevices", res)
	}

	var best *selectedDevice

	for _, d := range devices {
		var props C.VkPhysicalDeviceProperties
		C.vkGetPhysicalDeviceProperties(d, &props)

		var qCount C.uint32_t
		C.vkGetPhysicalDeviceQueueFamilyProperties(d, &qCount, nil)
		if qCount == 0 {
			continue
		}
		qProps := make([]C.VkQueueFamilyProperties, qCount)
		C.vkGetPhysicalDeviceQueueFamilyProperties(d, &qCount, &qProps[0])

		familyIdx := int32(-1)
		for i, q := range qProps {
			if q.queueFlags&C.VK_QUEUE_COMPUTE_BIT != 0 {
				familyIdx = int32(i)
				break
			}
		}
		if familyIdx < 0 {
			continue
		}

		var timestampPeriod float32
		if props.limits.timestampComputeAndGraphics != C.VK_FALSE && qProps[familyIdx].timestampValidBits > 0 {
			timestampPeriod = float32(props.limits.timestampPeriod)
		}

		cand := selectedDevice{
			device:          d,
			queueFamily:     C.uint32_t(familyIdx),
			name:            C.GoString(&props.deviceName[0]),
			discrete:        props.deviceType == C.VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU,
			timestampPeriod: timestampPeriod,
		}
		if best == nil || (cand.discrete && !best.discrete) {
			c := cand
			best = &c
		}
	}

	if best == nil {
		return selectedDevice{}, fmt.Errorf("no Vulkan device with a compute queue found")
	}
	return *best, nil
}

// deviceLocalMemoryBytes sums the memory heaps Vulkan marks device-local —
// dedicated VRAM on a discrete GPU, or the GPU-reserved portion of system
// RAM on an integrated one.
func deviceLocalMemoryBytes(physDevice C.VkPhysicalDevice) uint64 {
	var memProps C.VkPhysicalDeviceMemoryProperties
	C.vkGetPhysicalDeviceMemoryProperties(physDevice, &memProps)

	var total uint64
	for i := C.uint32_t(0); i < memProps.memoryHeapCount; i++ {
		heap := memProps.memoryHeaps[i]
		if heap.flags&C.VK_MEMORY_HEAP_DEVICE_LOCAL_BIT != 0 {
			total += uint64(heap.size)
		}
	}
	return total
}

func (c *ctx) createDevice(physDevice C.VkPhysicalDevice) error {
	priority := cnew(C.float(1.0))
	defer freeAll(unsafe.Pointer(priority))

	qci := cnew(C.VkDeviceQueueCreateInfo{
		sType:            C.VK_STRUCTURE_TYPE_DEVICE_QUEUE_CREATE_INFO,
		queueFamilyIndex: c.queueFamily,
		queueCount:       1,
		pQueuePriorities: priority,
	})
	defer freeAll(unsafe.Pointer(qci))

	var dci C.VkDeviceCreateInfo
	dci.sType = C.VK_STRUCTURE_TYPE_DEVICE_CREATE_INFO
	dci.queueCreateInfoCount = 1
	dci.pQueueCreateInfos = qci

	if res := C.vkCreateDevice(physDevice, &dci, nil, &c.device); res != C.VK_SUCCESS {
		return vkErr("vkCreateDevice", res)
	}
	C.gountlet_vk_load_device(c.device)
	C.vkGetDeviceQueue(c.device, c.queueFamily, 0, &c.queue)
	return nil
}

func (c *ctx) createBuffer(physDevice C.VkPhysicalDevice) error {
	const bufBytes = C.VkDeviceSize(numElements * 4)

	var bci C.VkBufferCreateInfo
	bci.sType = C.VK_STRUCTURE_TYPE_BUFFER_CREATE_INFO
	bci.size = bufBytes
	bci.usage = C.VK_BUFFER_USAGE_STORAGE_BUFFER_BIT
	bci.sharingMode = C.VK_SHARING_MODE_EXCLUSIVE

	if res := C.vkCreateBuffer(c.device, &bci, nil, &c.buffer); res != C.VK_SUCCESS {
		return vkErr("vkCreateBuffer", res)
	}

	var req C.VkMemoryRequirements
	C.vkGetBufferMemoryRequirements(c.device, c.buffer, &req)

	var memProps C.VkPhysicalDeviceMemoryProperties
	C.vkGetPhysicalDeviceMemoryProperties(physDevice, &memProps)

	const need = C.VkMemoryPropertyFlags(C.VK_MEMORY_PROPERTY_HOST_VISIBLE_BIT | C.VK_MEMORY_PROPERTY_HOST_COHERENT_BIT)
	typeIndex := int32(-1)
	for i := C.uint32_t(0); i < memProps.memoryTypeCount; i++ {
		bitSet := req.memoryTypeBits&(1<<i) != 0
		flags := memProps.memoryTypes[i].propertyFlags
		if bitSet && flags&need == need {
			typeIndex = int32(i)
			break
		}
	}
	if typeIndex < 0 {
		return fmt.Errorf("no host-visible+coherent memory type for compute buffer")
	}

	var mai C.VkMemoryAllocateInfo
	mai.sType = C.VK_STRUCTURE_TYPE_MEMORY_ALLOCATE_INFO
	mai.allocationSize = req.size
	mai.memoryTypeIndex = C.uint32_t(typeIndex)

	if res := C.vkAllocateMemory(c.device, &mai, nil, &c.memory); res != C.VK_SUCCESS {
		return vkErr("vkAllocateMemory", res)
	}
	if res := C.vkBindBufferMemory(c.device, c.buffer, c.memory, 0); res != C.VK_SUCCESS {
		return vkErr("vkBindBufferMemory", res)
	}

	var mapped unsafe.Pointer
	if res := C.vkMapMemory(c.device, c.memory, 0, bufBytes, 0, &mapped); res != C.VK_SUCCESS {
		return vkErr("vkMapMemory", res)
	}
	data := unsafe.Slice((*float32)(mapped), numElements)
	for i := range data {
		data[i] = 1.0
	}
	C.vkUnmapMemory(c.device, c.memory)
	return nil
}

func (c *ctx) createPipeline() error {
	code := C.CBytes(shaderSPV)
	defer freeAll(code)

	var smci C.VkShaderModuleCreateInfo
	smci.sType = C.VK_STRUCTURE_TYPE_SHADER_MODULE_CREATE_INFO
	smci.codeSize = C.size_t(len(shaderSPV))
	smci.pCode = (*C.uint32_t)(code)
	if res := C.vkCreateShaderModule(c.device, &smci, nil, &c.shaderModule); res != C.VK_SUCCESS {
		return vkErr("vkCreateShaderModule", res)
	}

	binding := cnew(C.VkDescriptorSetLayoutBinding{
		binding:         0,
		descriptorType:  C.VK_DESCRIPTOR_TYPE_STORAGE_BUFFER,
		descriptorCount: 1,
		stageFlags:      C.VK_SHADER_STAGE_COMPUTE_BIT,
	})
	defer freeAll(unsafe.Pointer(binding))

	var dslci C.VkDescriptorSetLayoutCreateInfo
	dslci.sType = C.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_LAYOUT_CREATE_INFO
	dslci.bindingCount = 1
	dslci.pBindings = binding
	if res := C.vkCreateDescriptorSetLayout(c.device, &dslci, nil, &c.setLayout); res != C.VK_SUCCESS {
		return vkErr("vkCreateDescriptorSetLayout", res)
	}

	setLayout := cnew(c.setLayout)
	defer freeAll(unsafe.Pointer(setLayout))
	pushRange := cnew(C.VkPushConstantRange{
		stageFlags: C.VK_SHADER_STAGE_COMPUTE_BIT,
		offset:     0,
		size:       4, // one uint32
	})
	defer freeAll(unsafe.Pointer(pushRange))

	var plci C.VkPipelineLayoutCreateInfo
	plci.sType = C.VK_STRUCTURE_TYPE_PIPELINE_LAYOUT_CREATE_INFO
	plci.setLayoutCount = 1
	plci.pSetLayouts = setLayout
	plci.pushConstantRangeCount = 1
	plci.pPushConstantRanges = pushRange
	if res := C.vkCreatePipelineLayout(c.device, &plci, nil, &c.pipelineLayout); res != C.VK_SUCCESS {
		return vkErr("vkCreatePipelineLayout", res)
	}

	entryPoint := C.CString("main")
	defer freeAll(unsafe.Pointer(entryPoint))

	var cpci C.VkComputePipelineCreateInfo
	cpci.sType = C.VK_STRUCTURE_TYPE_COMPUTE_PIPELINE_CREATE_INFO
	cpci.stage.sType = C.VK_STRUCTURE_TYPE_PIPELINE_SHADER_STAGE_CREATE_INFO
	cpci.stage.stage = C.VK_SHADER_STAGE_COMPUTE_BIT
	cpci.stage.module = c.shaderModule
	cpci.stage.pName = entryPoint
	cpci.layout = c.pipelineLayout
	if res := C.vkCreateComputePipelines(c.device, nil, 1, &cpci, nil, &c.pipeline); res != C.VK_SUCCESS {
		return vkErr("vkCreateComputePipelines", res)
	}

	poolSize := cnew(C.VkDescriptorPoolSize{
		_type:           C.VK_DESCRIPTOR_TYPE_STORAGE_BUFFER,
		descriptorCount: 1,
	})
	defer freeAll(unsafe.Pointer(poolSize))

	var dpci C.VkDescriptorPoolCreateInfo
	dpci.sType = C.VK_STRUCTURE_TYPE_DESCRIPTOR_POOL_CREATE_INFO
	dpci.maxSets = 1
	dpci.poolSizeCount = 1
	dpci.pPoolSizes = poolSize
	if res := C.vkCreateDescriptorPool(c.device, &dpci, nil, &c.descriptorPool); res != C.VK_SUCCESS {
		return vkErr("vkCreateDescriptorPool", res)
	}

	var dsai C.VkDescriptorSetAllocateInfo
	dsai.sType = C.VK_STRUCTURE_TYPE_DESCRIPTOR_SET_ALLOCATE_INFO
	dsai.descriptorPool = c.descriptorPool
	dsai.descriptorSetCount = 1
	dsai.pSetLayouts = setLayout
	if res := C.vkAllocateDescriptorSets(c.device, &dsai, &c.descriptorSet); res != C.VK_SUCCESS {
		return vkErr("vkAllocateDescriptorSets", res)
	}

	bufferInfo := cnew(C.VkDescriptorBufferInfo{
		buffer: c.buffer,
		offset: 0,
		_range: C.VK_WHOLE_SIZE,
	})
	defer freeAll(unsafe.Pointer(bufferInfo))

	var write C.VkWriteDescriptorSet
	write.sType = C.VK_STRUCTURE_TYPE_WRITE_DESCRIPTOR_SET
	write.dstSet = c.descriptorSet
	write.dstBinding = 0
	write.descriptorCount = 1
	write.descriptorType = C.VK_DESCRIPTOR_TYPE_STORAGE_BUFFER
	write.pBufferInfo = bufferInfo
	C.vkUpdateDescriptorSets(c.device, 1, &write, 0, nil)

	return nil
}

func (c *ctx) createCommands() error {
	var cpci C.VkCommandPoolCreateInfo
	cpci.sType = C.VK_STRUCTURE_TYPE_COMMAND_POOL_CREATE_INFO
	cpci.flags = C.VK_COMMAND_POOL_CREATE_RESET_COMMAND_BUFFER_BIT
	cpci.queueFamilyIndex = c.queueFamily
	if res := C.vkCreateCommandPool(c.device, &cpci, nil, &c.commandPool); res != C.VK_SUCCESS {
		return vkErr("vkCreateCommandPool", res)
	}

	var cbai C.VkCommandBufferAllocateInfo
	cbai.sType = C.VK_STRUCTURE_TYPE_COMMAND_BUFFER_ALLOCATE_INFO
	cbai.commandPool = c.commandPool
	cbai.level = C.VK_COMMAND_BUFFER_LEVEL_PRIMARY
	cbai.commandBufferCount = 1
	if res := C.vkAllocateCommandBuffers(c.device, &cbai, &c.commandBuffer); res != C.VK_SUCCESS {
		return vkErr("vkAllocateCommandBuffers", res)
	}

	var fci C.VkFenceCreateInfo
	fci.sType = C.VK_STRUCTURE_TYPE_FENCE_CREATE_INFO
	if res := C.vkCreateFence(c.device, &fci, nil, &c.fence); res != C.VK_SUCCESS {
		return vkErr("vkCreateFence", res)
	}

	if c.timestampPeriod > 0 {
		var qpci C.VkQueryPoolCreateInfo
		qpci.sType = C.VK_STRUCTURE_TYPE_QUERY_POOL_CREATE_INFO
		qpci.queryType = C.VK_QUERY_TYPE_TIMESTAMP
		qpci.queryCount = 2
		if res := C.vkCreateQueryPool(c.device, &qpci, nil, &c.queryPool); res != C.VK_SUCCESS {
			return vkErr("vkCreateQueryPool", res)
		}
	}
	return nil
}

// dispatch records, submits, and waits for a compute dispatch running
// `iterations` FMA passes per element, returning the GPU-side elapsed time.
func (c *ctx) dispatch(iterations uint32) (time.Duration, error) {
	if res := C.vkResetCommandBuffer(c.commandBuffer, 0); res != C.VK_SUCCESS {
		return 0, vkErr("vkResetCommandBuffer", res)
	}
	if res := C.vkResetFences(c.device, 1, &c.fence); res != C.VK_SUCCESS {
		return 0, vkErr("vkResetFences", res)
	}

	var begin C.VkCommandBufferBeginInfo
	begin.sType = C.VK_STRUCTURE_TYPE_COMMAND_BUFFER_BEGIN_INFO
	begin.flags = C.VK_COMMAND_BUFFER_USAGE_ONE_TIME_SUBMIT_BIT
	if res := C.vkBeginCommandBuffer(c.commandBuffer, &begin); res != C.VK_SUCCESS {
		return 0, vkErr("vkBeginCommandBuffer", res)
	}

	useTimestamps := c.queryPool != nil
	if useTimestamps {
		C.vkCmdResetQueryPool(c.commandBuffer, c.queryPool, 0, 2)
		C.vkCmdWriteTimestamp(c.commandBuffer, C.VK_PIPELINE_STAGE_TOP_OF_PIPE_BIT, c.queryPool, 0)
	}

	C.vkCmdBindPipeline(c.commandBuffer, C.VK_PIPELINE_BIND_POINT_COMPUTE, c.pipeline)
	C.vkCmdBindDescriptorSets(c.commandBuffer, C.VK_PIPELINE_BIND_POINT_COMPUTE, c.pipelineLayout, 0, 1, &c.descriptorSet, 0, nil)
	itersC := C.uint32_t(iterations)
	C.vkCmdPushConstants(c.commandBuffer, c.pipelineLayout, C.VK_SHADER_STAGE_COMPUTE_BIT, 0, 4, unsafe.Pointer(&itersC))
	C.vkCmdDispatch(c.commandBuffer, C.uint32_t(numElements/localSizeX), 1, 1)

	if useTimestamps {
		C.vkCmdWriteTimestamp(c.commandBuffer, C.VK_PIPELINE_STAGE_BOTTOM_OF_PIPE_BIT, c.queryPool, 1)
	}

	if res := C.vkEndCommandBuffer(c.commandBuffer); res != C.VK_SUCCESS {
		return 0, vkErr("vkEndCommandBuffer", res)
	}

	commandBuffer := cnew(c.commandBuffer)
	defer freeAll(unsafe.Pointer(commandBuffer))

	var submit C.VkSubmitInfo
	submit.sType = C.VK_STRUCTURE_TYPE_SUBMIT_INFO
	submit.commandBufferCount = 1
	submit.pCommandBuffers = commandBuffer

	start := time.Now()
	if res := C.vkQueueSubmit(c.queue, 1, &submit, c.fence); res != C.VK_SUCCESS {
		return 0, vkErr("vkQueueSubmit", res)
	}
	if res := C.vkWaitForFences(c.device, 1, &c.fence, C.VK_TRUE, ^C.uint64_t(0)); res != C.VK_SUCCESS {
		return 0, vkErr("vkWaitForFences", res)
	}

	if useTimestamps {
		var timestamps [2]C.uint64_t
		res := C.vkGetQueryPoolResults(
			c.device, c.queryPool, 0, 2,
			C.size_t(unsafe.Sizeof(timestamps)), unsafe.Pointer(&timestamps[0]), C.VkDeviceSize(8),
			C.VK_QUERY_RESULT_64_BIT|C.VK_QUERY_RESULT_WAIT_BIT,
		)
		if res == C.VK_SUCCESS {
			ticks := uint64(timestamps[1] - timestamps[0])
			return time.Duration(float64(ticks) * float64(c.timestampPeriod)), nil
		}
		// Fall through to wall-clock timing below if the query somehow
		// failed despite the device claiming timestamp support.
	}
	return time.Since(start), nil
}
