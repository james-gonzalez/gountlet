//go:build cgo

// Package gpu benchmarks GPU compute throughput by dispatching a Vulkan
// compute shader that performs a large number of fused multiply-adds and
// timing the dispatch. It links directly against the system's Vulkan
// loader (libvulkan.so.1 / vulkan-1.dll / libvulkan.dylib), so no display
// or windowing system is required. The Vulkan headers are vendored under
// vk/include (see vk/LICENSE) so the build needs only the loader, not a
// full Vulkan SDK.
package gpu

/*
#cgo CFLAGS: -I${SRCDIR}/vk/include
#cgo linux LDFLAGS: -lvulkan
#cgo darwin LDFLAGS: -lvulkan
#cgo windows LDFLAGS: -lvulkan-1
#include <vulkan/vulkan_core.h>
#include <stdlib.h>
#include <string.h>

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
	targetSecs  = 1.5
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
	deviceName     string
}

// destroy releases every handle that was successfully created, in reverse order.
func (c *ctx) destroy() {
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
// shader, plus reports the selected device's name.
func Run() bench.Result {
	name := "gpu"
	c := &ctx{}
	defer c.destroy()

	if res := C.gountlet_create_instance(&c.instance); res != C.VK_SUCCESS {
		return bench.Fail(name, vkErr("vkCreateInstance", res))
	}

	physDevice, queueFamily, deviceName, discrete, err := pickDevice(c.instance)
	if err != nil {
		return bench.Fail(name, err)
	}
	c.queueFamily = queueFamily
	c.deviceName = deviceName
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
	// then scale up to hit roughly targetSecs of GPU work.
	calibElapsed, err := c.dispatch(calibIters)
	if err != nil {
		return bench.Fail(name, err)
	}
	itersPerSec := float64(calibIters) / calibElapsed.Seconds()
	targetIters := uint32(itersPerSec * targetSecs)
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
	if discrete {
		deviceKind = "discrete"
	}

	res := bench.Result{Name: name}
	res.Add("compute", gflops, "GFLOPS", deviceKind+" GPU")
	res.AddInfo("device", c.deviceName)
	if vramBytes > 0 {
		res.AddInfo("vram", bench.FormatBytes(vramBytes))
	}
	return res
}

// pickDevice enumerates physical devices and returns the first discrete GPU
// with a compute-capable queue family, falling back to any such device.
func pickDevice(instance C.VkInstance) (device C.VkPhysicalDevice, queueFamily C.uint32_t, name string, discrete bool, err error) {
	var count C.uint32_t
	if res := C.vkEnumeratePhysicalDevices(instance, &count, nil); res != C.VK_SUCCESS {
		return nil, 0, "", false, vkErr("vkEnumeratePhysicalDevices", res)
	}
	if count == 0 {
		return nil, 0, "", false, fmt.Errorf("no Vulkan physical devices found")
	}
	devices := make([]C.VkPhysicalDevice, count)
	if res := C.vkEnumeratePhysicalDevices(instance, &count, &devices[0]); res != C.VK_SUCCESS {
		return nil, 0, "", false, vkErr("vkEnumeratePhysicalDevices", res)
	}

	type candidate struct {
		device      C.VkPhysicalDevice
		queueFamily C.uint32_t
		name        string
		discrete    bool
	}
	var best *candidate

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

		cand := candidate{
			device:      d,
			queueFamily: C.uint32_t(familyIdx),
			name:        C.GoString(&props.deviceName[0]),
			discrete:    props.deviceType == C.VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU,
		}
		if best == nil || (cand.discrete && !best.discrete) {
			c := cand
			best = &c
		}
	}

	if best == nil {
		return nil, 0, "", false, fmt.Errorf("no Vulkan device with a compute queue found")
	}
	return best.device, best.queueFamily, best.name, best.discrete, nil
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

	C.vkCmdBindPipeline(c.commandBuffer, C.VK_PIPELINE_BIND_POINT_COMPUTE, c.pipeline)
	C.vkCmdBindDescriptorSets(c.commandBuffer, C.VK_PIPELINE_BIND_POINT_COMPUTE, c.pipelineLayout, 0, 1, &c.descriptorSet, 0, nil)
	itersC := C.uint32_t(iterations)
	C.vkCmdPushConstants(c.commandBuffer, c.pipelineLayout, C.VK_SHADER_STAGE_COMPUTE_BIT, 0, 4, unsafe.Pointer(&itersC))
	C.vkCmdDispatch(c.commandBuffer, C.uint32_t(numElements/localSizeX), 1, 1)

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
	return time.Since(start), nil
}
